---
id: plugins-isolated-web-app-contributions-design
title: Isolated plugin web-application contributions system design
status: draft
system: plugins
owners:
  - kandev
created: 2026-08-26
last_updated: 2026-08-30
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-001
  - REQ-PLUGINS-ISOLATED-WEB-APPS-002
  - REQ-PLUGINS-ISOLATED-WEB-APPS-003
  - REQ-PLUGINS-ISOLATED-WEB-APPS-004
  - REQ-PLUGINS-ISOLATED-WEB-APPS-005
  - REQ-PLUGINS-ISOLATED-WEB-APPS-006
  - REQ-PLUGINS-ISOLATED-WEB-APPS-007
  - REQ-PLUGINS-ISOLATED-WEB-APPS-008
  - REQ-PLUGINS-ISOLATED-WEB-APPS-009
  - REQ-PLUGINS-ISOLATED-WEB-APPS-010
  - REQ-PLUGINS-ISOLATED-WEB-APPS-011
---

# Isolated plugin web-application contributions system design

## Purpose and boundaries

The Plugins system adds an isolated `web_app` contribution beside the trusted
native frontend bundle. A web application uses packaged static files in a
sandboxed iframe. It does not run in the Kandev SPA process.

This design owns plugin instances, immutable releases, capability grants,
runtime tokens, web data access, shared instance state, and live browser
events. The Canvases system owns canvas scope changes and user workflows.

This design implements
[ADR-2026-08-26-plugin-backed-web-app-canvases](../../../decisions/2026-08-26-plugin-backed-web-app-canvases.md).
It extends the data boundary from
[ADR 0043](../../../decisions/0043-plugin-host-data-api.md).

## Requirement mapping

| Requirement                         | Design sections                                  |
| ----------------------------------- | ------------------------------------------------ |
| `REQ-PLUGINS-ISOLATED-WEB-APPS-001` | Web-application contribution, Browser host       |
| `REQ-PLUGINS-ISOLATED-WEB-APPS-002` | Package and release contract, Release activation |
| `REQ-PLUGINS-ISOLATED-WEB-APPS-003` | Instance grants, Request authorization           |
| `REQ-PLUGINS-ISOLATED-WEB-APPS-004` | Browser data protocol                            |
| `REQ-PLUGINS-ISOLATED-WEB-APPS-005` | Shared instance state                            |
| `REQ-PLUGINS-ISOLATED-WEB-APPS-006` | Live event protocol                              |
| `REQ-PLUGINS-ISOLATED-WEB-APPS-007` | Browser security boundary                        |
| `REQ-PLUGINS-ISOLATED-WEB-APPS-008` | Plugin instance model, Compatibility             |
| `REQ-PLUGINS-ISOLATED-WEB-APPS-009` | Diagnostics and observability                    |
| `REQ-PLUGINS-ISOLATED-WEB-APPS-010` | Artifact storage, Protocol compatibility         |
| `REQ-PLUGINS-ISOLATED-WEB-APPS-011` | Host appearance protocol                         |

## Existing plugin contracts

The implementation extends these current boundaries:

- `internal/plugins/manifest` owns manifest parsing and validation.
- `internal/plugins/pkgtar` owns safe plugin package extraction.
- `internal/plugins` owns installation, activation, actions, Host data, state,
  events, and agent tools.
- `pkg/pluginsdk` and `proto/kandev/plugin/v1/plugin.proto` own the managed
  backend contract.
- `apps/packages/plugin-sdk` owns the trusted native frontend contract.
- `lib/plugins` loads native bundles and stores frontend contributions.

The new web-application runtime does not use `PluginHostApi` or
`PluginRegistry`. Existing native bundles keep those interfaces unchanged.

## Web-application contribution

Extend `manifest.UISection` with `web_apps`:

```yaml
ui:
  web_apps:
    - key: main
      title: Task board
      entry: ui/index.html
      placements: [task-canvas, workspace-canvas]
      network_origins: [https://api.example.com]
```

`key` is unique inside the plugin. `entry` is a package-relative HTML file.
The first placement vocabulary contains `task-canvas` and
`workspace-canvas`. `network_origins` is an optional list of exact HTTPS
origins for this web application. It cannot contain paths, credentials,
wildcards, query strings, or fragments. Later specifications can add other
host placements.

The host owns the displayed navigation label, icon, scope, route, and actions.
The iframe owns only its document rectangle.

A package can contain these contribution combinations:

| Package form           | Native bundle | Web application | Managed backend |
| ---------------------- | ------------- | --------------- | --------------- |
| Existing native plugin | optional      | none            | required        |
| Static canvas package  | none          | required        | none            |
| Managed web plugin     | optional      | required        | required        |

A web-application-only package omits `runtime`. A managed plugin keeps
`runtime.type: binary` and can use declared authenticated actions.

## Plugin instance model

Installed plugin records stay the source for package identity and managed
backend lifecycle. Add explicit plugin instances for scoped activation.

### `plugin_instances`

- `id`
- `plugin_id`
- `source_kind` as `installed` or `local_canvas`
- `scope_kind` as `instance`, `workspace`, `task`, `session`, or `repository`
- nullable trusted scope identifiers
- `status` as `pending`, `active`, `disabled`, `archived`, `error`, or `removed`
- nullable `active_release_id`
- monotonic `grant_generation` incremented on scope, release, status, or grant
  changes
- timestamps

Only identifiers required by `scope_kind` can be set. A database constraint and
service validation reject incomplete or mixed scopes.

Existing installed plugins receive one implicit `instance` scope. This
compatibility projection does not change current settings, enablement, process
lifecycle, or native UI loading.

### `plugin_releases`

- `id`
- `plugin_id`
- `package_digest`
- `source_kind`
- `source_actor_kind`
- nullable source user, task, and session identifiers
- `manifest_json`
- `declared_permissions_json`
- `artifact_path`
- `validation_status`
- nullable safe validation error
- `created_at`

Release rows and extracted artifacts are immutable. The artifact path is under
the Kandev data directory. It is not an agent workspace path.

### `plugin_instance_grants`

- `plugin_instance_id`
- `permission_kind`
- `resource`
- optional exact HTTPS origin
- trusted scope ceiling
- approving user and time

The service normalizes grants before persistence. It rejects a grant that the
active or pending release does not declare.

## Package and release contract

A local web application package contains the manifest, source, built static
files, and optional metadata. Kandev serves only files under the declared web
root. It stores the source so an edit agent can create the next draft.

The agent builds any TypeScript, React, or other source in its task executor.
Kandev does not install dependencies or run package scripts during validation.
Simple canvases can use direct HTML, CSS, and JavaScript without a build step.

Initial package limits are:

- 10 MiB compressed package
- 25 MiB expanded package
- 512 files
- 5 MiB for one file
- 64 KiB manifest
- 240 bytes for one normalized path

Retained artifacts use these initial storage limits:

- 2 GiB for one workspace
- 10 GiB for one Kandev installation
- one active valid release, one prior valid release, and one pending release
  for each plugin instance

The admission transaction reserves the compressed artifact bytes before it
moves an artifact into immutable storage. A concurrent publish cannot exceed a
workspace or installation limit. A rejection does not create a release row or
change the active release.

The validator accepts HTML, CSS, JavaScript, JSON, common web images, fonts,
and source-map files. It uses content sniffing and file extensions. It rejects
device files, links, duplicate normalized paths, traversal, absolute paths,
and files outside the package root.

The capability path is rooted at the declared entry directory for ordinary
relative asset requests. For example, `ui/index.html` can load `./app.js` and
`./app.css` from `ui/`. The reserved `_kandev/v1` path remains rooted at the
capability URL and is dispatched before ordinary asset mapping.

`pkgtar` provides the extraction pattern. Web packages use a separate validator
because they can omit a managed backend executable.

## Release activation

Release activation uses these steps:

1. Stream the bounded package into a temporary directory.
2. Validate all package paths and sizes before extraction.
3. Parse and validate the complete manifest.
4. Validate each declared web application and its entry document.
5. Compute the package digest and declared permission set.
6. Move the validated artifact into immutable release storage.
7. Create the release row.
8. Compare the permission set with the current instance grants.
9. Activate the release in one transaction when no new grant is required.
10. Publish one instance lifecycle event after commit.

If the release needs a new grant, the service records it as pending. The active
release stays unchanged. A user can approve the permissions and activate the
pending release in one transaction.

The initial retention rule keeps the active release and one prior valid
release for each local canvas instance. A pending release is also retained
until approval, replacement, or removal.

## Instance grants

The effective permission calculation is:

```text
manifest declaration
  intersect instance grant
  intersect trusted instance scope
  intersect current caller authorization
```

The permission vocabulary reuses current manifest capabilities:

- `api_read`
- `api_write`
- `events`
- `state`
- declared authenticated `actions`

Add exact HTTPS origin grants for browser network access. Each web application
declares its own `network_origins`, so one app cannot inherit another app's
external origins. Remote script origins are not grantable in this version.

Task-scoped instances default to the current task as their scope ceiling.
Workspace access requires a separate approved grant. Promotion performs a new
review because it changes the scope ceiling.

## Browser host

Add one host component for web applications. The component requests a runtime
URL for an authorized instance, release, placement, and user. The backend
returns a short-lived capability URL.

The component renders an iframe with these rules:

- `sandbox` permits scripts and forms.
- The sandbox omits `allow-same-origin`, top navigation, and popups.
- The iframe does not receive Kandev cookies or authorization headers.
- The host does not inject a JavaScript object or use a privileged message
  bridge. It can send the presentation-only appearance envelope below.
- The host chrome contains status, navigation, permissions, releases, and
  lifecycle actions.

The runtime response also applies the sandbox through the CSP `sandbox`
directive. This directive protects a capability URL that a person opens as a
top-level document. The response permits only `allow-scripts` and
`allow-forms`. It does not permit `allow-same-origin`, and `form-action
'none'` denies form submissions to external origins.

The desktop shell adds a narrow `frame-src` rule for the loopback backend. The
runtime `frame-ancestors` policy permits the configured web host,
`tauri://localhost`, and `http://tauri.localhost`. It denies other parents.
Desktop packaging tests cover both Tauri origin forms and direct browser use.

The iframe can use `prefers-color-scheme` and responsive CSS as fallbacks. The
host appearance protocol supplies the exact active Kandev semantic colors.

## Host appearance protocol

The isolated runtime has one host-to-frame presentation message. Version 1 has
this shape:

```json
{
  "type": "kandev.web_app.appearance",
  "version": 1,
  "mode": "dark",
  "tokens": {
    "background": "...",
    "foreground": "...",
    "card": "...",
    "cardForeground": "...",
    "muted": "...",
    "mutedForeground": "...",
    "border": "...",
    "primary": "...",
    "primaryForeground": "...",
    "accent": "...",
    "accentForeground": "...",
    "destructive": "...",
    "destructiveForeground": "...",
    "ring": "..."
  }
}
```

`WebAppFrame` resolves these values from the current host theme. Each value is
a bounded serialized CSS color from a fixed key allowlist. The envelope
contains no identity, capability, data, storage, navigation, or action field.

After the iframe load event, the host sends the initial envelope to that
iframe's `contentWindow`. The loading cover remains for one animation frame so
the application can apply the values before it becomes visible. The host sends
another envelope when the resolved Kandev theme changes. It does not reload the
iframe.

The opaque iframe has no stable origin, so the host targets its exact window
with `targetOrigin: "*"`. The application listener accepts only messages whose
source is `window.parent` and whose type, version, mode, keys, and value bounds
match the contract. This source check prevents a sibling frame from setting
appearance values. The wildcard does not grant authority because the payload
contains public presentation data only.

The bundled scaffold maps the token keys to documented CSS custom properties.
It includes safe light and dark fallbacks. An application can ignore this
message, but it does not receive another theme API or a privileged reply
channel.

## Runtime token

The runtime URL contains a random capability segment. The backend stores only
a digest of this value. The capability expires after 15 minutes and can use a
bounded sliding renewal while the host page remains authorized.

The capability binds these values:

- user identity
- plugin instance
- active release
- web-application key
- placement
- scope kind and trusted resource identifiers
- grant generation

Each request loads the current instance and compares these bindings. A scope,
grant, release, status, or access change revokes the old capability on its next
request.

Approved external origins are a deliberate direct-browser exception to this
per-request runtime check. Kandev cannot inspect a request after the browser
has sent it to a third-party origin. The WebSocket lifecycle broadcaster sends
an authority-change notification to the host, and the host unmounts the
matching iframe immediately. The replacement iframe is mounted only after a
fresh HTTP metadata and runtime-binding load. This is the revocation boundary
for direct external requests. Kandev runtime and protocol requests still run
the binding validator on every request.

The runtime route accepts the capability without an ambient session cookie.
The route sets explicit CORS responses for the sandboxed opaque origin. It
does not accept the token from a general authorization header or query field.

## Browser data protocol

The capability path is the application base. The application uses relative
requests. Example paths are:

```text
GET    ./_kandev/v1/context
GET    ./_kandev/v1/data/tasks
GET    ./_kandev/v1/data/tasks/{id}
PATCH  ./_kandev/v1/data/tasks/{id}
POST   ./_kandev/v1/data/tasks/{id}/messages
GET    ./_kandev/v1/data/workflows
GET    ./_kandev/v1/data/workflows/{id}/steps
GET    ./_kandev/v1/state
GET    ./_kandev/v1/state/{key}
PUT    ./_kandev/v1/state/{key}
DELETE ./_kandev/v1/state/{key}
GET    ./_kandev/v1/events
POST   ./_kandev/v1/actions/{key}
```

`context` returns only public instance metadata, scope identifiers that the
application can already use, active release identity, and effective
capabilities. It does not return a user token, Kandev session, filesystem path,
secret, or private plugin configuration.

The data routes reuse the DTOs, filters, opaque pagination, and conservative
write fields from the Plugin Host data API. The shared task projection adds
`workflow_step_id`. The browser and gRPC adapters return this field with the
same nullability and authorization rules.

The task message route reuses the Plugin Host `SendMessage` service. It accepts
a bounded prompt and an optional session ID. The runtime context supplies the
plugin instance, user, workspace, and source attribution. A static canvas uses
this route for actions such as sending `continue` to a task.

Extract shared application services from `internal/plugins/host_data_*` when
the current gRPC handler owns logic that both adapters need.

All writes use Kandev domain services. They never write a repository directly.
The service publishes the same task and workflow events as first-party writes.

Initial browser request limits are 256 KiB for a request and 1 MiB for a
response page. Each request has a 30-second deadline and supports cancellation.

## Shared instance state

Add `plugin_instance_state` or extend `plugin_state` with a non-null instance
identity. The migration must preserve current instance-global plugin state.

Each entry contains:

- plugin instance
- state key
- JSON value
- monotonic revision
- update time
- safe writer kind

The browser uses `If-Match` with the current revision for an update or removal.
A stale request returns `409 plugin_state_conflict` with the current revision.
The response does not include the current value unless the caller still has
read permission.

Canvas agent tools use the same state service. They derive the plugin instance
from trusted canvas and session context.

## Live event protocol

The browser opens the relative Server-Sent Events endpoint. The event gateway
subscribes to declared Kandev event subjects and filters every event through the
current instance scope.

The stream sends public, bounded DTOs. It never forwards internal event objects
or unfiltered plugin bus payloads.

Each stream has a process generation and monotonic sequence. A bounded
in-memory ring retains recent events for active instances. The browser sends
`Last-Event-ID` after reconnect.

The initial stream contract uses these operational limits:

- one heartbeat comment every 15 seconds
- immediate header and event flushing
- at most two streams for one user and instance
- at most 20 streams for one user
- at most 1,000 retained events or five minutes of replay for one instance

The response sets `Cache-Control: no-cache, no-transform` and
`X-Accel-Buffering: no`. The server releases subscriptions and counters when
the request context ends. `EventSource` cannot set an authorization header, so
the event endpoint uses the capability path and accepts no query token.

The gateway sends `runtime.resync_required` when the generation changed or the
sequence is no longer available. The application then reloads its authoritative
data and state snapshot. Domain data and plugin state remain the sources of
truth.

## Managed plugin actions

A managed plugin can declare a web application and authenticated actions. The
relative action route reuses the authorization, scope verification, deadlines,
body limits, status projection, and cancellation contract from
`POST /api/plugins/:id/actions/:key`.

A static web-application-only package has no custom action handler. Its browser
code uses the Host data and state routes. A later backend-runtime decision can
add another handler type without changing the browser protocol.

## Browser security boundary

The runtime document response uses this minimum header policy:

- `Content-Security-Policy` includes `sandbox allow-scripts allow-forms`,
  `default-src 'none'`, `form-action 'none'`, `base-uri 'none'`, and
  `object-src 'none'`
- `frame-ancestors` contains only normalized Kandev web and Tauri host origins
- `X-Content-Type-Options` is `nosniff`
- `Referrer-Policy` is `no-referrer`
- `Cross-Origin-Resource-Policy` is `cross-origin` because the document has an
  opaque origin and Tauri is a separate origin
- capability-bearing HTML and API responses use `Cache-Control: no-store`

The response uses a restrictive resource policy:

- packaged and inline application scripts can run
- dynamic code evaluation is denied
- remote scripts are denied
- styles and packaged assets can load
- data images can load within a bounded policy
- data connections can reach the runtime API
- approved HTTPS origins can receive explicit `connect-src`, image, or font
  access while the iframe binding is current
- direct external requests are not server-proxied in this version. Lifecycle
  authority notifications tear down the iframe before a replacement load
- framing is limited to the Kandev host

The implementation must build the policy from normalized grants. It must not
copy manifest text into a response header.

The runtime strips plugin-supplied framing, cookie, authentication, service
worker, and cross-origin policy headers. Service workers are unavailable under
the opaque origin and runtime path.

The host must treat all iframe content, titles, errors, downloads, and links as
untrusted. The iframe cannot cover host controls or navigate the top frame.

The authoring guide states that an opaque-origin application cannot use
`localStorage`, `sessionStorage`, IndexedDB, or service workers. The application
uses instance state for durable shared values and memory for temporary values.

## Compatibility

The native plugin frontend remains an in-process trusted extension. A package
must declare `ui.bundle` to use that path. A `ui.web_apps` declaration does not
receive `host.api`, `host.storage`, `host.context`, `host.ui`, or registry
methods.

The managed backend gRPC contract remains unchanged for a static web
application. Managed plugins can combine both frontend contribution types.

The plugin list and settings API add instance and web-application projections
without removing current installed-plugin fields.

The `./_kandev/v1` browser protocol remains available while any retained
release declares version 1. A later protocol version uses a new relative path.
Kandev does not rewrite an immutable release. If a future Kandev version cannot
serve a retained protocol, startup marks the release unavailable before it can
execute and the host shows edit, rollback, or removal actions.

## Artifact storage and recovery

Validated artifacts remain immutable files under the Kandev data directory.
The database stores their digest, relative path, byte count, and availability.
The artifact directory is part of the Kandev-home recovery boundary.

SQLite database snapshots do not contain release artifacts. PostgreSQL backups
also require a backup of the Kandev artifact directory. Public operations and
configuration documentation state this boundary.

Startup reconciles retained release rows with the artifact inventory before it
registers runtime routes. A missing file, digest mismatch, or unsafe path marks
the release `plugin_release_unavailable`. Reconciliation never executes or
silently removes the artifact. It records a content-free diagnostic and leaves
the release metadata available for recovery.

Workspace and instance removal create durable artifact-cleanup jobs in the
same transaction that removes release ownership. A worker removes the files
after commit and retries after restart. This order preserves cleanup inventory
if the process stops between the database change and file removal.

## Failure and recovery

- An invalid package never enters release storage as a valid release.
- An activation transaction publishes no event before commit.
- A stale runtime token returns `runtime_token_stale`.
- A revoked grant returns `plugin_permission_denied`.
- A stale state write returns `plugin_state_conflict`.
- A lost event range returns `runtime.resync_required`.
- A missing active release returns `plugin_release_unavailable`.
- An unavailable managed backend disables its action routes but does not block
  static asset and Host data routes.

The browser host maps stable codes to localized messages. It never shows raw
backend or package errors.

## Observability

Add counters and duration metrics for:

- package validation and activation result
- runtime capability issue and rejection result
- data resource and operation result
- state operation and conflict result
- event connection, replay, and resync result
- content-policy and network denial result
- artifact bytes admitted, retained, rejected, unavailable, and removed

Logs can contain plugin ID, instance ID, release ID, web-application key,
resource type, operation, safe result, and duration. Logs omit source, content,
state values, bodies, payloads, runtime capabilities, and user credentials.

## Test strategy

- Manifest tests cover web-application declarations and combinations.
- Package tests cover traversal, links, duplicate paths, limits, and digest
  stability.
- Repository tests cover instances, releases, grants, state revisions, and
  migration replay on SQLite and PostgreSQL.
- Service tests cover release activation and grant intersection.
- HTTP tests cover capability binding, stale tokens, CORS, limits, and stable
  errors.
- Security tests cover sandbox flags, content policy, form-action denial,
  remote script denial, cookie absence, nested asset resolution, authority
  revocation after load, and top-navigation denial.
- Host data contract tests compare browser and gRPC projections.
- Event tests cover scope filtering, reconnect replay, generation changes, and
  resync.
- Frontend component tests cover host state and iframe lifecycle.
- Appearance tests cover initial delivery, source validation, token bounds,
  live changes, and computed colors in direct, Dockview, and phone hosts.

## Related decisions

- [Plugin-backed web-app canvases](../../../decisions/2026-08-26-plugin-backed-web-app-canvases.md)
- [Plugin Host data API](../../../decisions/0043-plugin-host-data-api.md)
- [Authenticated plugin actions](../../../decisions/2026-07-31-authenticated-plugin-actions.md)
- [Plugin contribution lifecycle](../../../decisions/2026-08-04-plugin-contribution-lifecycle-authority.md)
- [Plugin agent tools through Kandev MCP](../../../decisions/2026-08-11-plugin-tools-through-kandev-mcp.md)
