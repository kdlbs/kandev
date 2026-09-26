---
id: plugins-isolated-web-app-contributions
title: Plugin web-application contributions
status: draft
system: plugins
owners:
  - kandev
created: 2026-08-26
last_updated: 2026-09-21
---

# Plugin web-application contributions Requirements

## Overview

Plugins provide packaged HTML, CSS, and JavaScript in a separate document.
The selected same-origin mode treats this code as trusted by the viewing user.
It does not provide browser isolation from the Kandev host.

The [same-origin decision](../../../decisions/2026-09-19-trusted-same-origin-canvases.md)
amends the original isolation contract. The runtime and proxy-verification
implementation is complete under the [delivery plan](../../../plans/canvas-same-origin-auth/plan.md).
The existing document path and requirement IDs remain stable.

The Plugins system owns package validation, releases, instances, grants,
runtime tokens, data access, state access, event delivery, and browser trust.

## Terminology

- **Web application:** Packaged static HTML, CSS, JavaScript, fonts, and images.
- **Plugin instance:** One enabled binding of a plugin package to an instance,
  workspace, task, session, or repository scope.
- **Grant:** A subset of declared permissions approved directly by the user or
  through [owner-authorized canvas creation](../../canvases/requirements/local-creation-authority.md).
- **Runtime token:** A short-lived capability that authorizes one iframe
  instance independently of ambient Kandev session cookies.
- **Native bundle:** A trusted React module that runs inside the Kandev SPA.

## Requirements

### REQ-PLUGINS-ISOLATED-WEB-APPS-001: Isolated web-application contribution

**Intent:** A plugin can provide an arbitrary web interface in a separate document
without importing its code as a native frontend bundle.

#### Acceptance criteria

- **AC-PLUGINS-ISOLATED-WEB-APPS-001.1:** A plugin manifest shall declare one
  or more web applications with a stable key, title, entry document, and
  supported placements.
- **AC-PLUGINS-ISOLATED-WEB-APPS-001.2:** Kandev shall render each web
  application in a sandboxed iframe and shall not import it as an ES module.
- **AC-PLUGINS-ISOLATED-WEB-APPS-001.3:** The host shall not inject its React
  instance, Zustand store, native plugin API, or credential values into the iframe.
  Same-origin browser access follows `REQ-PLUGINS-ISOLATED-WEB-APPS-013`; this
  criterion does not promise host-DOM or session isolation.
- **AC-PLUGINS-ISOLATED-WEB-APPS-001.4:** The contribution shall support
  packaged HTML, CSS, JavaScript, images, and fonts within documented limits.
- **AC-PLUGINS-ISOLATED-WEB-APPS-001.5:** Existing native plugin bundles shall
  keep their current trusted runtime and registration contract.

### REQ-PLUGINS-ISOLATED-WEB-APPS-002: Validated immutable releases

**Intent:** Runtime content has a known package, digest, and validation result.

#### Acceptance criteria

- **AC-PLUGINS-ISOLATED-WEB-APPS-002.1:** Before activation, Kandev shall
  validate the manifest, entry document, paths, file types, file count,
  compressed size, expanded size, and individual file sizes.
- **AC-PLUGINS-ISOLATED-WEB-APPS-002.2:** The validator shall reject absolute
  paths, traversal, symbolic links, duplicate paths, unsupported files, and
  content that exceeds a limit.
- **AC-PLUGINS-ISOLATED-WEB-APPS-002.3:** Each accepted release shall have an
  immutable content digest and source provenance.
- **AC-PLUGINS-ISOLATED-WEB-APPS-002.4:** Activation shall replace the active
  release atomically after complete validation.
- **AC-PLUGINS-ISOLATED-WEB-APPS-002.5:** A failed activation shall preserve
  the active release, grants, instance state, and navigation contributions.
- **AC-PLUGINS-ISOLATED-WEB-APPS-002.6:** A supported Kandev backup shall state
  whether it contains release artifacts, and a restore shall not execute a
  release whose artifact is missing.

### REQ-PLUGINS-ISOLATED-WEB-APPS-003: Effective capability grants

**Intent:** Requests through the capability protocol receive only the declared,
granted, and authorized scope. This limit does not constrain direct requests
that trusted same-origin code makes through the ordinary user session.

#### Acceptance criteria

- **AC-PLUGINS-ISOLATED-WEB-APPS-003.1:** The effective permission set shall be
  the intersection of package declarations, instance grants, and current
  Kandev authorization.
- **AC-PLUGINS-ISOLATED-WEB-APPS-003.2:** Each capability-protocol data, state,
  event, and action request shall revalidate the active instance and effective
  permission set. A direct browser request to an approved external origin
  cannot be intercepted by Kandev after it leaves the browser. The host shall
  therefore tear down the iframe immediately after every authority-changing
  lifecycle notification, and a new iframe shall receive a newly validated
  binding before it can make another request.
- **AC-PLUGINS-ISOLATED-WEB-APPS-003.3:** When an instance becomes disabled,
  archived, removed, or inaccessible, its existing runtime tokens shall stop
  authorizing new Kandev requests, and the host shall tear down any matching
  iframe immediately. In-flight direct external requests are not treated as
  newly authorized requests.
- **AC-PLUGINS-ISOLATED-WEB-APPS-003.4:** A new release shall not gain a
  permission without direct user approval or recorded owner-authorized initial
  canvas creation. Package-provided identity shall not establish that authority.
- **AC-PLUGINS-ISOLATED-WEB-APPS-003.5:** A denial shall return a stable safe
  code without resource content or existence details.

### REQ-PLUGINS-ISOLATED-WEB-APPS-004: Standard web data protocol

**Intent:** A web application reads and changes Kandev data without an injected
JavaScript SDK.

#### Acceptance criteria

- **AC-PLUGINS-ISOLATED-WEB-APPS-004.1:** Kandev shall expose a versioned
  relative HTTP API for the data resources that the effective grant permits.
- **AC-PLUGINS-ISOLATED-WEB-APPS-004.2:** The HTTP data shapes and write fields
  shall reuse the stable Plugin Host data contract.
- **AC-PLUGINS-ISOLATED-WEB-APPS-004.3:** Data writes shall use Kandev service
  methods that publish the normal domain events.
- **AC-PLUGINS-ISOLATED-WEB-APPS-004.4:** Kandev shall not inject a global
  canvas or plugin JavaScript object into the iframe.
- **AC-PLUGINS-ISOLATED-WEB-APPS-004.5:** Browser input shall not supply a
  trusted user, plugin instance, workspace, task, session, or repository
  context.
- **AC-PLUGINS-ISOLATED-WEB-APPS-004.6:** Requests shall use bounded bodies,
  responses, pagination, deadlines, and cancellation.
- **AC-PLUGINS-ISOLATED-WEB-APPS-004.7:** A task projection shall include the
  current workflow step when the caller can read that task.
- **AC-PLUGINS-ISOLATED-WEB-APPS-004.8:** A granted web application shall send
  a task prompt through the same message service as the Plugin Host API.

### REQ-PLUGINS-ISOLATED-WEB-APPS-005: Shared state and conflict control

**Intent:** The user interface and authorized agents can use durable canvas
state without browser-local ownership.

#### Acceptance criteria

- **AC-PLUGINS-ISOLATED-WEB-APPS-005.1:** A granted web application shall read,
  list, set, and remove JSON state in its plugin-instance scope.
- **AC-PLUGINS-ISOLATED-WEB-APPS-005.2:** Each state entry shall include a
  revision or equivalent write precondition.
- **AC-PLUGINS-ISOLATED-WEB-APPS-005.3:** A stale state write shall return the
  current revision and shall not overwrite the current value.
- **AC-PLUGINS-ISOLATED-WEB-APPS-005.4:** Authorized agent tools shall read and
  change the same plugin-instance state through the backend contract.
- **AC-PLUGINS-ISOLATED-WEB-APPS-005.5:** The capability state API shall not let
  one plugin instance read or change another instance's state.

### REQ-PLUGINS-ISOLATED-WEB-APPS-006: Live events and recovery

**Intent:** A web application stays current when users, agents, integrations,
or automations change Kandev data.

#### Acceptance criteria

- **AC-PLUGINS-ISOLATED-WEB-APPS-006.1:** A granted web application shall
  receive its declared and authorized Kandev events through a versioned
  Server-Sent Events endpoint.
- **AC-PLUGINS-ISOLATED-WEB-APPS-006.2:** Each event shall include a stable
  identifier, type, scope identity, and bounded public payload.
- **AC-PLUGINS-ISOLATED-WEB-APPS-006.3:** After a disconnect, the application
  shall reconnect with the last event identifier or reload an authoritative
  data snapshot.
- **AC-PLUGINS-ISOLATED-WEB-APPS-006.4:** Event delivery shall filter each event
  by the current instance scope and effective grants.
- **AC-PLUGINS-ISOLATED-WEB-APPS-006.5:** Event history shall not become the
  source of truth for tasks, workflows, repositories, or plugin state.
- **AC-PLUGINS-ISOLATED-WEB-APPS-006.6:** The event endpoint shall use bounded
  connection counts, replay storage, heartbeat intervals, and stream lifetime.

### REQ-PLUGINS-ISOLATED-WEB-APPS-007: Browser security boundary

**Intent:** The runtime keeps explicit content and framing policies while
allowing the trusted same-origin behavior in requirement 013. These policies
are not containment against code that can access the host document.

#### Acceptance criteria

- **AC-PLUGINS-ISOLATED-WEB-APPS-007.1:** The iframe shall retain its served
  origin. Its declared sandbox shall permit scripts, forms, and same-origin
  behavior, without adding top-navigation or popup permissions.
- **AC-PLUGINS-ISOLATED-WEB-APPS-007.2:** Runtime requests shall use a
  short-lived user-bound, instance-bound, release-bound, and scope-bound token.
- **AC-PLUGINS-ISOLATED-WEB-APPS-007.3:** The capability API shall require a
  valid runtime token even when a request carries browser session cookies.
  Cookies shall not expand that token's effective grants.
- **AC-PLUGINS-ISOLATED-WEB-APPS-007.4:** The content policy shall allow
  packaged scripts and styles but deny remote scripts, undeclared network
  origins, and form submissions.
- **AC-PLUGINS-ISOLATED-WEB-APPS-007.5:** A network permission shall name exact
  HTTPS origins and shall require direct or delegated user approval under
  `AC-PLUGINS-ISOLATED-WEB-APPS-003.4`.
- **AC-PLUGINS-ISOLATED-WEB-APPS-007.6:** The host shall keep navigation,
  permission, release, archive, edit, and remove controls outside the iframe.
- **AC-PLUGINS-ISOLATED-WEB-APPS-007.7:** Each runtime document response shall
  apply the same declared sandbox permissions when a person opens its capability
  URL outside an iframe. It shall not claim an opaque-origin boundary.
- **AC-PLUGINS-ISOLATED-WEB-APPS-007.8:** Supported web and desktop hosts shall
  frame runtime documents, and every other ancestor shall be denied.
- **AC-PLUGINS-ISOLATED-WEB-APPS-007.9:** The runtime shall resolve ordinary
  relative asset requests beside a nested entry document while reserving the
  `_kandev/v1` path at the capability root.
- **AC-PLUGINS-ISOLATED-WEB-APPS-007.10:** A web host and runtime served from
  the same origin shall support custom DNS names, IP addresses, and ports
  without per-host configuration. Distinct origins shall not acquire embedding
  permission through request headers or wildcard matching.

### REQ-PLUGINS-ISOLATED-WEB-APPS-008: Scoped instance lifecycle

**Intent:** One plugin package can have a bounded instance with task or
workspace authority without changing instance-global installed plugins.

#### Acceptance criteria

- **AC-PLUGINS-ISOLATED-WEB-APPS-008.1:** A plugin instance shall have one
  scope kind and the required trusted resource identifiers for that scope.
- **AC-PLUGINS-ISOLATED-WEB-APPS-008.2:** Instance enable, disable, scope
  change, release activation, grant change, and removal shall update discovery
  without a Kandev restart.
- **AC-PLUGINS-ISOLATED-WEB-APPS-008.3:** Existing installed plugins shall use
  an implicit global instance until they adopt explicit instances.
- **AC-PLUGINS-ISOLATED-WEB-APPS-008.4:** A web-application-only package shall
  not require a managed plugin backend process.
- **AC-PLUGINS-ISOLATED-WEB-APPS-008.5:** A managed plugin can declare the same
  web-application contribution and keep its existing backend lifecycle.

### REQ-PLUGINS-ISOLATED-WEB-APPS-009: Runtime diagnostics

**Intent:** Operators and users can diagnose package, permission, token, data,
state, and event errors without exposing application content.

#### Acceptance criteria

- **AC-PLUGINS-ISOLATED-WEB-APPS-009.1:** Metrics shall count package
  validation, activation, token, data, state, event, and policy results.
- **AC-PLUGINS-ISOLATED-WEB-APPS-009.2:** Logs can include plugin, instance,
  release, resource type, operation, result code, and duration.
- **AC-PLUGINS-ISOLATED-WEB-APPS-009.3:** Logs and metrics shall omit source
  files, HTML, script, state values, request bodies, event payloads, tokens, and
  data content.
- **AC-PLUGINS-ISOLATED-WEB-APPS-009.4:** Validation errors shall identify safe
  file paths and rules without returning file contents.

### REQ-PLUGINS-ISOLATED-WEB-APPS-010: Bounded storage and protocol support

**Intent:** Web applications cannot exhaust release storage or lose their
runtime protocol while Kandev retains their releases.

#### Acceptance criteria

- **AC-PLUGINS-ISOLATED-WEB-APPS-010.1:** Kandev shall enforce documented
  workspace and installation limits for retained release artifacts.
- **AC-PLUGINS-ISOLATED-WEB-APPS-010.2:** A storage-limit rejection shall not
  activate a release or change the current grants and instance state.
- **AC-PLUGINS-ISOLATED-WEB-APPS-010.3:** Kandev shall support the browser API
  version that a retained release uses, or mark that release unavailable with
  recovery actions.
- **AC-PLUGINS-ISOLATED-WEB-APPS-010.4:** When release metadata references a
  missing artifact, Kandev shall mark the release unavailable before runtime
  execution.
- **AC-PLUGINS-ISOLATED-WEB-APPS-010.5:** Storage metrics shall report bounded
  counts and bytes without package names, paths, or content.

### REQ-PLUGINS-ISOLATED-WEB-APPS-011: Host appearance context

**Intent:** A web application matches the current Kandev appearance through
a bounded presentation message.

#### Acceptance criteria

- **AC-PLUGINS-ISOLATED-WEB-APPS-011.1:** Before Kandev reveals an isolated web
  application, the host shall provide its resolved light or dark mode and a
  bounded set of semantic colors.
- **AC-PLUGINS-ISOLATED-WEB-APPS-011.2:** When the user changes the Kandev
  theme, an open web application shall receive the new appearance without a
  browser reload.
- **AC-PLUGINS-ISOLATED-WEB-APPS-011.3:** The appearance contract shall be
  versioned, presentation-only, and shall not provide Kandev data, identity,
  credentials, host APIs, or DOM access.
- **AC-PLUGINS-ISOLATED-WEB-APPS-011.4:** The same appearance behavior shall
  apply to direct desktop routes, task panels, and phone hosts.

### REQ-PLUGINS-ISOLATED-WEB-APPS-012: Runtime startup acknowledgement

**Intent:** A host can distinguish a started document from a blocked or failed
frame through the existing bounded startup message.

#### Acceptance criteria

- **AC-PLUGINS-ISOLATED-WEB-APPS-012.1:** The runtime shall acknowledge
  startup only after the document loads and its current capability context
  succeeds. A captured startup script or asset failure shall report failure.
- **AC-PLUGINS-ISOLATED-WEB-APPS-012.2:** The host shall accept startup
  messages only from the current frame attempt. Sibling, malformed, unsupported,
  and stale messages shall not change host status.
- **AC-PLUGINS-ISOLATED-WEB-APPS-012.3:** Startup messages shall contain no
  credentials, runtime URLs, application content, or domain actions. They shall
  not authorize data access, grant changes, navigation, or host APIs.
- **AC-PLUGINS-ISOLATED-WEB-APPS-012.4:** Existing retained HTML releases shall
  gain startup detection without republishing or changing their stored bytes or
  digest. A startup timeout shall preserve releases, grants, and stored state.

### REQ-PLUGINS-ISOLATED-WEB-APPS-013: Trusted same-origin authentication

**Intent:** A user can open a trusted canvas behind a cookie-authenticated
reverse proxy without a canvas-specific authentication bypass.

#### Acceptance criteria

- **AC-PLUGINS-ISOLATED-WEB-APPS-013.1:** When the host and runtime share an
  origin, startup and default relative API requests shall send eligible browser
  cookies. The iframe and response sandbox policies shall both preserve that origin.
- **AC-PLUGINS-ISOLATED-WEB-APPS-013.2:** When a proxy requires a valid session
  cookie, an authenticated user shall reach Ready and read permitted canvas data.
  The same behavior shall apply to task panels, direct routes, and phone hosts.
- **AC-PLUGINS-ISOLATED-WEB-APPS-013.3:** A cookie shall not replace a missing,
  expired, revoked, or invalid runtime capability. Capability permission denials
  and backend input validation shall remain effective with cookies present.
- **AC-PLUGINS-ISOLATED-WEB-APPS-013.4:** Documentation shall state that canvas
  code is trusted with the viewing user's browser authority. It can access the
  same-origin host DOM and storage, and use ordinary user-authorized APIs.
  Canvas grants shall not be described as a complete security boundary.
- **AC-PLUGINS-ISOLATED-WEB-APPS-013.5:** Retained releases using default relative
  fetches shall gain this behavior without republishing or changing stored bytes.
  Explicit credential omission shall remain an application choice.
- **AC-PLUGINS-ISOLATED-WEB-APPS-013.6:** When proxy authentication expires,
  startup shall fail through the existing recoverable state. After authentication
  is restored, Retry shall obtain a fresh runtime and reach Ready.
- **AC-PLUGINS-ISOLATED-WEB-APPS-013.7:** Cookie-authenticated relative writes
  and event subscriptions shall work through the same public origin. External
  origins shall not gain cookie forwarding, CORS access, or framing permission.
- **AC-PLUGINS-ISOLATED-WEB-APPS-013.8:** Runtime document, packaged asset, and
  host bootstrap responses shall prohibit intermediary transformation while
  retaining their no-store policy. A proxy that honors this prohibition shall
  deliver a working retained release without injected analytics or republishing.
  Startup errors and CSP restrictions shall retain their existing behavior.

## Implementation plans

- [Canvas runtime and task-entry recovery](../../../plans/canvas-runtime-entry-recovery/plan.md)

## Out of scope

- A second JavaScript SDK for canvases.
- A supported API for direct host-DOM or frontend-store manipulation. Same-origin
  code can access these browser surfaces, but they are not stable author contracts.
- Remote script execution.
- An unbounded general proxy to Kandev HTTP endpoints.
- Automatic grants without direct or recorded delegated user approval.
- A host process for agent-generated backend code.
- General-purpose plugin placement in every Kandev UI surface.
- A general bidirectional iframe SDK or privileged message bridge.
