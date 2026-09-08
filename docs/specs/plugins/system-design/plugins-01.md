---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-PLUGINS-001
created: 2026-04-26
owners:
  - cfl
---
# Plugin System System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLUGINS-PLUGINS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-PLUGINS-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

The plugin system is a **platform-level capability of kandev**, not tied to any single
feature area. It lets third parties (and kandev itself) extend the product — backend
behavior and native frontend UI — through a stable contract, without forking or
modifying core.

The gRPC/go-plugin transport, package format, and RPC surface described below are
frozen in `docs/plans/plugins/GRPC-CONTRACT.md`; that file is the authoritative
wire-level reference. The native frontend contract is frozen in
`docs/plans/plugins/PLUGIN-API.md`. This spec describes the resulting product
behavior.

## Why

Kandev keeps growing external integrations and surface-specific behavior directly in
the core codebase: source-control sync, issue-tracker browsing, notification providers,
and planned channel types (Slack, Discord, Telegram, email). Each one adds
platform-specific logic — API clients, webhook handlers, payload formatting, OAuth
flows, secret management, and bespoke UI — to the Go backend and the SPA. This creates
three problems:

1. **Core bloat.** Every new integration increases the surface area of core. Adding more
   at similar scale makes the codebase unmaintainable.
2. **Release coupling.** Fixing a bug in one integration requires a full kandev release,
   and users who don't use it still receive the code. Integration authors cannot ship
   independently of the core release cycle.
3. **Extensibility ceiling.** Users and third parties cannot add their own integrations
   or UI without forking kandev.

A plugin system decouples extensions from core. Plugin **backends** are Go binaries
that kandev spawns and supervises as subprocesses, communicating over a strict typed
gRPC protocol; plugins may additionally ship a **native frontend bundle** that kandev
loads into the SPA. Both extend kandev through well-defined, capability-gated
surfaces. The core stays small; the ecosystem grows independently.

## What

- Plugin **backends** are **Go binaries** distributed inside a release tarball
  (per-platform executables) that kandev **spawns and supervises as subprocesses**
  via `hashicorp/go-plugin`, speaking a strict typed **gRPC protocol**
  (`kandev.plugin.v1`) over a unix domain socket (macOS/Linux) or loopback TCP with
  AutoMTLS (Windows). No in-process backend loading, no separately-managed operator
  process, no HTTP transport for the backend contract.
- A plugin MAY additionally ship a **native frontend bundle** (`ui.bundle`) that kandev
  loads into the SPA to register native routes/nav/components (see "Frontend plugin
  runtime"). This is the one in-process surface; the backend stays out-of-process
  (but is now kandev-managed, not operator-managed).
- A plugin manifest declares identity, runtime executables (per OS/arch), capabilities,
  declared webhooks and authenticated actions, repository-provider and reference-source
  ownership, config schema, and optional UI bundle.
- Plugins SHALL receive events, expose proxied external webhook
  endpoints, and read/write a plugin-scoped KV state — all over gRPC.
- Plugins are distributed as a signed-or-unsigned release **tarball** and installed
  either by **URL** (kandev downloads it) or by **manual upload** (multipart file).
  There is no manifest-paste registration step.
- The Settings > Plugins install dialog SHALL show the primary Install action as
  busy while an install is in flight, including an animated loading indicator and
  an installing label, while keeping the action disabled until the pipeline settles.
- Capability-based access control: a plugin can only call Host RPCs it declared in its
  manifest; undeclared capabilities are rejected with a gRPC `PermissionDenied` status.
- **Kandev owns the plugin process lifecycle**: it extracts the package, spawns the
  binary, performs the go-plugin handshake, health-checks it (`Ping`), and restarts it
  on crash or health-check failure. Operators no longer run or manage plugin processes
  themselves. The remote/self-hosted tier (`base_url` registration of an
  operator-run process kandev never spawns) is removed; see "Out of scope".
- Plugin repository providers, task actions, and review panels register through
  revocable generic frontend contracts. A provider plugin can participate in native
  task creation, Link menus, desktop/mobile review surfaces, and composer `#` search
  without a host-specific provider branch.
- The independently consumable, runtime-free `@kandev/plugin-sdk` package is the
  frontend author contract. Official plugins use its typed `host.context` reads and
  named UI/provider contracts instead of importing or copying private Kandev store,
  React, Zustand, or `@/` application types.
- First-party-parity code-host dashboards use host-owned provider-neutral list,
  toolbar, scope, task-menu, and linked-task primitives exposed through `host.ui`.
  Plugins provide normalized data and callbacks; task presets open the native
  `TaskCreateDialog`, while review remains owned by the task review-provider surface.
- Native UI plugins register bounded, plugin-owned translation catalogs and consume them through a
  host-scoped imperative/reactive localization API. Locale changes follow Kandev without bundling a
  second i18n runtime, and disable/reload/uninstall removes only that plugin's resources.

## Data model

### Plugin package format

A plugin ships as a release tarball, `<id>-<version>.tar.gz`:

```
manifest.yaml                        # authoritative; read BEFORE any code runs
server/plugin-<goos>-<goarch>[.exe]  # any subset of platforms; host key required at install
ui/bundle.js                         # optional (frontend half)
ui/*.css / assets/icon.svg           # optional
checksums.txt                        # "sha256  path" for every other file
checksums.txt.sig                    # OPTIONAL ed25519 signature (unsigned → warn, not blocked)
```

`manifest.yaml` declares identity, capabilities, webhooks, config schema, and
an optional UI bundle, plus a `runtime` block naming the per-platform executables
(replaces the old `base_url`/`endpoints` block, which is removed entirely):

```yaml
id: "kandev-plugin-slack"                    # Unique, pattern: ^[a-z0-9][a-z0-9._-]*$
api_version: 1
version: "1.0.0"
display_name: "Slack Notifications"
description: "Post to Slack on task events, relay messages to agents"
author: "kandev"
categories: ["connector"]                    # connector | automation | tools | analytics

runtime:
  type: binary
  executables:
    linux-amd64: server/plugin-linux-amd64
    darwin-arm64: server/plugin-darwin-arm64
    # ... any subset; kandev requires the running host's platform key at install time
min_kandev_version: "0.91.1"                 # required for admin actions

capabilities:
  events: ["task.created", "task.state_changed", "agent.completed"]
  api_read: ["tasks", "sessions"]             # Host data API reads (see below); live now
  api_write: ["tasks", "messages"]            # Host data API writes (CreateTask/UpdateTask/SendMessage); live now
  state: true
  secrets: true
  auth: true                                  # establish a login session for an
                                              # external (OIDC/SAML) identity — see ADR 0050
  user_state: true                            # authenticated, per-user host.storage — see
                                              # "Frontend plugin runtime" and ADR
                                              # 2026-08-01-per-user-plugin-storage

actions:
  - key: "connection.save"
    scope: workspace
    access: admin                             # defaults to authenticated
    max_body_bytes: 65536

repository_providers: ["example-repository-provider"]

reference_sources:
  - source: "example_change_requests"
    provider: "example-repository-provider"
    kind: "change_request"
    display_name: "Example change requests"
    kind_label: "Change request"
    order: 100

webhooks:
  - key: "slack-events"
    description: "Slack Events API webhook"
    method: "POST"
    access: public

config_schema:
  type: object
  properties:
    bot_token_secret: { type: string, description: "Secret reference for Slack bot token" }
    default_channel:  { type: string, description: "Default channel for notifications" }
    notify_on_task_created: { type: boolean, default: true }
  required: ["bot_token_secret", "default_channel"]

ui:                                            # Native frontend plugin (see "Frontend plugin runtime")
  bundle: "/ui/bundle.js"                      # root-relative ES module path
  styles: ["/ui/plugin.css"]                   # optional root-relative stylesheets

# Runtime fields managed by kandev (not authored):
status: "active"
version: "1.0.0"
install_path: "~/.kandev/plugins/kandev-plugin-slack/1.0.0"
signed: false
installed_at: "2026-04-26T10:00:00Z"
restart_count: 0
last_error: null
last_error_at: null
```

`last_error` and `last_error_at` are host-managed runtime diagnostics. When a
spawn, handshake, health check, restart, or install-path check moves a plugin to
`error`, kandev stores a single-line diagnostic (bounded to 2048 bytes) and the
UTC timestamp of the failure. Before persistence, kandev redacts credential-like
values (including PATs, bearer tokens, and labeled passwords, tokens, secrets,
or API keys) and replaces the host home path with `~`; arbitrary plugin stdout
and untrusted subprocess details are never persisted verbatim. The diagnostic
is bounded after redaction. A successful handshake and health check clears both
fields. Existing records without these fields load as null.

`capabilities.api_read` / `capabilities.api_write` gate the **Host data API** Host
RPCs (both reads and writes live now) — the vocabulary is a list of
resource names: `tasks`, `sessions`, `messages`, `workspaces`, `workflows`,
`agent_profiles`, `executor_profiles`, `repositories` for `api_read`, plus `tasks` (CreateTask/
UpdateTask) and `messages` (SendMessage) for `api_write`. See "Host data API".

**Declaring data access.** Listing a resource under `api_read` grants the
corresponding Host data reads for that resource only — e.g. `api_read:
["sessions"]` unlocks `Host.Sessions().List(...)` and
`Host.Sessions().CodeStats(...)` (backed by `ListSessions` /
`ListSessionCodeStats`) but not `Host.Tasks()`. A resource left off the list
still resolves to a reader/accessor (no nil pointer), but every method on it
returns gRPC `PermissionDenied` with message `capability 'api_read:<resource>'
not declared`. Writes work the same way under `api_write` (message
`capability 'api_write:<resource>' not declared`), and reads and writes gate
independently on the same resource — a plugin may declare `api_read:tasks`
without `api_write:tasks`, or vice versa (see "Host data API writes").

`actions` declare the only browser-invocable plugin operations. Each key is unique per
plugin, has one resource scope (`workspace`, `task`, or `repository`), and supplies a
bounded request-body maximum. `repository_providers` declares stable IDs the plugin may
register in the frontend repository-provider registry; one enabled plugin owns one
provider ID. A provider that supports the native task branch picker declares the
workspace-scoped `repositories.branches` action. Kandev invokes it server-to-server with
the persisted repository identity and accepts only a bounded normalized branch list;
browser input cannot select the provider or repository for that invocation.
`reference_sources` declares plugin-owned composer sources. Source names
are unique, a descriptor cannot claim another provider, and all entries are removed when
the plugin disables. See ADRs
[2026-07-31-authenticated-plugin-actions](../../../decisions/2026-07-31-authenticated-plugin-actions.md)
and
[2026-07-31-plugin-repository-provider-extensions](../../../decisions/2026-07-31-plugin-repository-provider-extensions.md).

**External login (`auth`).** An `auth`-capable plugin can log a visitor in
against an external IdP (OIDC/SAML): its webhook (the callback / SAML ACS)
validates the token, then asserts the identity to kandev via the reserved
`X-Kandev-Auth-Login` response header (`{provider, subject, email,
display_name}`). The host maps it to a user (link-by-email or just-in-time
member provisioning), mints the session, and sets the session cookie (name
derived from the request host) itself — the plugin never receives the raw
token. Requires auth enabled; new
users are members, and the host never creates an admin nor auto-links to an
existing admin account. The plugin **must** only assert IdP-verified emails (an
unverified email claim is an account-takeover vector the host cannot detect).
This is the highest-privilege capability; grant it only to trusted plugins.
See [ADR 0050](../../../decisions/0050-plugin-external-auth-capability.md).

### Install pipeline

There is no manifest-paste registration step. A plugin is installed from a URL or an
uploaded tarball:

1. An administrator calls `POST /api/plugins/install` with JSON `{"url": "..."}`
   (kandev downloads the tarball) or a multipart `package` field (direct upload).
   Auth-disabled single-user instances retain their synthetic administrator.
2. Kandev verifies `checksums.txt` covers every other file in the tarball and every
   hash matches (integrity gate, always enforced).
3. If `checksums.txt.sig` is present, kandev verifies the ed25519 signature; if
   absent, install proceeds with a surfaced "unsigned plugin" warning (signing is not
   required in v1 — see "Out of scope").
4. Kandev parses and validates `manifest.yaml` **before any code runs**: schema, `id`
   pattern, capability vocabulary, and that `runtime.executables` contains an entry
   for the host's OS/arch.
5. Kandev extracts the package to `~/.kandev/plugins/<id>/<version>/` and records the
   installation (`id`, `version`, `install_path`, capabilities, status).
6. Kandev spawns the platform-matched binary via `hashicorp/go-plugin`, completes the
   handshake (§2 of GRPC-CONTRACT.md) — status `registered` while this is pending.
7. Handshake succeeds → status `active`. Handshake/spawn failure → status `error`
   (restart retried with backoff; see "State machine"). The failure diagnostic is
   persisted on the record so the operator can see why activation failed and
   retry with **Enable**.

Uninstall stops the subprocess and removes the record, all installed versions, and
plugin state (no 24-hour grace period in v1). `POST /api/plugins/register` is removed;
there is no operator-supplied manifest, no generated credentials, and no cleartext
secret returned at install time.

### `plugin_state` (SQLite)

```sql
CREATE TABLE plugin_state (
    id TEXT PRIMARY KEY,
    plugin_id TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'instance',    -- instance | workspace | task | agent
    scope_id TEXT,                              -- NULL for instance scope
    state_key TEXT NOT NULL,
    value_json TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (plugin_id, scope, scope_id, state_key)
);
```
