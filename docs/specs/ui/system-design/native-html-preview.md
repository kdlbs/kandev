---
status: current
system: ui
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
---

# Native HTML File Preview System Design

## Purpose and boundaries

Native HTML preview is a one-click static-server workflow built on Kandev's
existing browser iframe policy and session port proxy. It favors browser
fidelity over hostile-content isolation. Agentctl serves the current editor
buffer as an in-memory entry document and serves relative assets from the
selected task workspace. The active file viewer replaces its source body with
an iframe that renders the page with normal HTML, CSS, and JavaScript behavior.

The feature does not create a virtual browser, transform user scripts, or
introduce a second executor-routing protocol. It also does not make a security
claim for arbitrary workspace HTML. The user-selected trust model is recorded
in [ADR-2026-09-05-trusted-browser-html-preview](../../../decisions/2026-09-05-trusted-browser-html-preview.md).

## Requirement mapping

| Requirement                      | Design sections                                                                                                                                                                                                                                                               |
| -------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-NATIVE-HTML-PREVIEW-001` | [Architecture](#architecture), [Contracts](#contracts), [Control flow](#control-flow), [Trust and security](#trust-and-security), [Responsive contract](#responsive-contract), [Failure and recovery](#failure-and-recovery), [Verification strategy](#verification-strategy) |

## Architecture

```text
File editor current buffer
        |
        | POST session-scoped preview request
        v
Kandev backend task handler
        |
        | authenticated agentctl request
        v
Agentctl workspace preview manager
        |-- in-memory entry-document overlay
        |-- workspace-rooted static files
        `-- loopback ephemeral HTTP port
                     |
                     | existing session port proxy
                     v
          Desktop or mobile file-preview iframe
```

The existing gateway port proxy remains responsible for session authorization,
executor reachability, capability cookies for iframe subresources, response
rewriting, and Browser-panel compatibility. The preview feature adds no public
listener and no direct browser-to-agentctl connection.

## Components and responsibilities

### Agentctl

- A workspace preview manager starts one loopback HTTP server per selected root
  when it receives the first request. The manager closes every server during
  agentctl instance teardown. A port identifies one static root, including for
  multi-repository tasks.
- The manager stores entry-document overlays by canonical repository-relative
  path. Publishing the same path replaces its previous buffer and increments a
  version token.
- The static handler serves an overlay for an exact entry-document match and
  otherwise serves files from the selected workspace or repository root.
- Existing repository path resolution remains authoritative. Request paths are
  URL-decoded once, cleaned, joined through the workspace path helper, and
  rejected if the resolved path or symlink target escapes the selected root.
- Responses use standard MIME detection and `Cache-Control: no-store` so a
  republished current buffer and edited assets are visible on refresh.

### Backend

- A task-session handler authorizes the requested session and validates the
  request. It also makes sure that agentctl is ready. Then it forwards the
  request through the existing agentctl client.
- The agentctl client preserves non-success status codes in a typed error
  without retaining the response body. The task-session handler maps agentctl
  validation, size, and availability responses to 400, 413, and 503; other
  upstream failures and malformed success responses become 502.
- The backend does not store HTML or proxy asset bytes itself.
- The response contains the agentctl port, scoped entry path, and version needed
  to construct a session port-proxy URL.

### Frontend

- Preview eligibility remains derived from editable `.html` and `.htm` file
  paths after binary-file classification.
- The frontend API publishes the current editor buffer and converts the result
  into the same port-proxy URL form used by development servers.
- Desktop renders the URL through `HtmlPreviewContent` in place of the active
  source editor. `Show code` restores the source body, refresh republishes the
  latest buffer, and an explicit secondary action can call
  `openBrowserPanel(url)` without making that panel the primary outcome.
- Mobile renders the same URL in the focused file viewer and uses the same
  source-recovery, refresh, trust, loading, and error semantics.
- Markdown keeps its sanitized in-place renderer and existing preview state.

## Contracts

### Browser-to-backend request

```http
POST /api/v1/task-sessions/{sessionID}/html-previews
Content-Type: application/json

{
  "repo": "optional repository identity",
  "path": "relative/path/index.html",
  "content": "current editor buffer"
}
```

The request body is capped at 5 MiB. `path` must be an editable `.html` or
`.htm` file within the task workspace. `repo` uses the same optional multi-repo
identity accepted by existing workspace-file operations.

The successful response is:

```json
{
  "port": 43127,
  "path": "/relative/path/index.html",
  "version": 4
}
```

The frontend appends the version as a cache-busting query value when opening
the session port-proxy URL. The port is an implementation detail scoped to the
live task session, not a durable public identifier.

### Backend-to-agentctl request

```http
POST /api/v1/workspace/html-previews
Content-Type: application/json
```

The body and response use the same fields. Agentctl repeats size, extension,
and path validation because the backend is not its trust boundary. Existing
agentctl authentication applies.

### Overlay bounds

Each document is limited to 5 MiB. An agentctl instance retains at most 32
entry-document overlays across its root servers. Replacing an existing path
does not consume another slot. When the limit is reached, the least recently
published overlay is evicted. The number of loopback servers is bounded by the
task's configured workspace and repository roots. Overlay bytes, versions, and
access metadata live only in memory.

These limits constrain accidental memory growth. They are not a hostile-code
sandbox and do not constrain resources consumed by browser scripts.

## Control flow

### Desktop

1. The file editor derives HTML preview eligibility from the current path.
2. The user activates `Preview HTML` after seeing the trusted-code affordance.
3. The frontend sends the current buffer, path, and repository identity to the
   task-session endpoint without saving the file.
4. The backend authorizes the session and publishes the entry document through
   agentctl.
5. Agentctl starts or reuses its workspace preview server and returns its port,
   path, and new version.
6. The frontend constructs the existing session port-proxy URL and replaces the
   source editor body with `HtmlPreviewContent` at that URL.
7. The iframe loads the page. Relative subresources traverse the same proxy and
   are read from the workspace by agentctl.
8. `Refresh` republishes the latest unsaved buffer and loads the new versioned
   URL. `Show code` restores source, while `Open in Browser panel` explicitly
   opens the same URL as a secondary workflow.

The file tab, current buffer, and dirty state remain owned by the source editor
flow while the iframe is visible.

### Mobile

1. The focused file viewer exposes the same `Preview HTML` action.
2. Publishing follows the desktop flow.
3. The focused viewer replaces its code body with a full-height iframe at the
   returned proxy URL and keeps a `Show code` action outside the iframe.
4. `Show code` or file identity change restores source without changing its
   buffer. A preview failure keeps source recovery and retry actions available.

## Relative assets and browser fidelity

For an entry document at `site/pages/index.html`, `../styles/site.css` resolves
to `site/styles/site.css` and `/images/logo.svg` resolves from the selected
workspace or repository root. Query strings and fragments do not participate
in filesystem path resolution.

The static server does not rewrite HTML, CSS, JavaScript, or import maps.
Standard browser behavior covers scripts, modules, DOM APIs, fetch, media,
forms, canvas, WebGL, and available Browser-panel APIs. An application that requires a build
step, custom fallback routing, server-side endpoints, or HMR must continue to
use the configured development-server workflow.

## Trust and security

HTML preview uses the existing Browser-panel iframe policy, including scripts
and same-origin behavior. It is intentionally a trusted-code workflow.

The following remain required:

1. Preview HTML is never inserted into the Kandev parent DOM.
2. Browser access always traverses the session-authorized port proxy.
3. Static file reads stay within the selected workspace or repository root,
   including after symlink evaluation.
4. Agentctl listens only on loopback and stops the preview server with the
   instance.
5. The UI tells the user that previewing runs workspace code with Browser-panel
   capabilities.
6. Logs and errors do not include the HTML buffer.

These controls protect routing and host filesystem scope. They do not isolate
the user from malicious JavaScript. In deployments where Browser-panel content
shares an origin with Kandev, previewed code can exercise the authority that
origin and the Browser panel expose. Operators who cannot trust workspace HTML
must not enable or use this action until a dedicated-origin mode exists.

## Lifecycle and persistence

- A root's preview server starts on first publish and is reused for the
  agentctl instance lifetime.
- Entry overlays are in-memory only and do not save or modify workspace files.
- Leaving preview or closing the file tab unmounts the iframe but does not stop
  the shared server. Agentctl teardown stops the server.
- In-editor preview state and URLs are not persisted. Activating `Preview HTML`
  after reload or executor restart publishes the current buffer to the live
  instance and obtains a valid URL.
- Preview state is scoped to the active session plus repository-and-path
  identity and resets when that identity changes.

## Responsive contract

- **Desktop entry and outcome:** The existing file toolbar exposes `Preview
HTML`. The action replaces the source body inside the same file tab with an
  iframe and keeps `Show code`, refresh, and optional Browser-panel actions in
  Kandev-owned chrome.
- **Phone entry and outcome:** `MobileFileViewerPanel` exposes a minimum
  44-pixel action target. It shows a focused full-height iframe and puts `Show
code` in Kandev-owned chrome.
- **Nearest shipped exemplars:** Markdown preview supplies the in-place source
  toggle and file identity behavior. The mobile HTML focused viewer supplies
  the iframe toolbar, source recovery, and full-height composition. The
  development-server preview keeps ownership of automatic Browser-panel use.
- **Scroll ownership:** Browser content owns iframe scrolling. Kandev's focused
  viewer owns its header and must not add page-level horizontal overflow.
- **Shared behavior:** Eligibility, publication, errors, trust copy, proxy URL,
  refresh, source recovery, and retry semantics are shared. Only toolbar
  density and viewport composition differ.

## Failure and recovery

- Invalid paths, unsupported extensions, oversized buffers, and workspace-root
  escapes return validation errors and do not start or update a preview.
- A missing or stopped task session returns the existing session-unavailable
  response. The UI preserves source and shows localized retry guidance.
- An agentctl start or publish failure keeps the in-editor preview surface open
  with localized retry and `Show code` actions. Retry republishes the latest
  buffer.
- A missing relative asset receives a normal HTTP 404 inside the preview so the
  browser's native diagnostics remain meaningful.
- A stale URL after agentctl restart is not restored as active preview state.
  Republishing obtains the current ephemeral port and version.
- The UI never falls back to `srcDoc`, QuickJS, a virtual DOM, or direct file
  URLs when the server path fails.

## Compatibility and migration

Remove the QuickJS worker, virtual-DOM renderer, source-navigation normalizer,
and their dependencies. Existing persisted `markdownPreview` compatibility only
applies to Markdown. HTML in-place preview starts from source after reload and
publishes only after the user activates it.

The explicit development-server action remains unchanged. Native HTML preview
is the zero-configuration static case, while the development-server path
remains the correct choice for applications with build or backend behavior.

## Observability

Agentctl emits structured lifecycle logs for preview-server start, stop, and
publish failures. Logs can include the canonical relative path, response status,
and byte count, but not source content. Existing port-proxy logs and metrics
continue to cover browser routing.

## Verification strategy

- Agentctl unit tests cover server start, reuse, shutdown, concurrent
  publish/read, per-root ports, GET/HEAD/405 method handling, overlays, disk
  assets, MIME types, no-store headers, path traversal, symlink escape, and
  missing files.
- Backend handler and client tests cover session authorization, agentctl
  readiness, payload bounds, forwarding, typed 400/413/503 propagation,
  malformed requests, unavailable sessions, malformed responses, and response
  validation.
- Frontend API and component tests cover unsaved-buffer publication, in-editor
  source/preview toggling, explicit Browser-panel opening, versioned URLs,
  trusted-code copy, errors, retry, refresh, source-state preservation, identity
  reset, and stale publish completion guards.
- Desktop Chromium E2E proves that an unsaved document runs native JavaScript
  and loads relative CSS, JavaScript, and images. It also proves use of a native
  browser API and refresh after a second publish.
- Mobile Chrome E2E proves the same proxied content, 44-pixel action, full-height
  containment, `Show code`, and source preservation.
- Existing development-server and Browser-panel tests guard against regressions
  in explicit server workflows and generic port-proxy behavior.

## Open implementation risks

- Every executor lifecycle path must own the agentctl preview server. This
  ownership prevents ephemeral port leaks after stop or restart.
- Root-relative and module subresource requests must retain the session proxy
  capability cookie through redirects and rewritten URLs.
- Multi-repository tasks must use the same repository identity and path helper
  as existing workspace-file APIs.
- Preview publication completion must remain scoped to the file and session
  that initiated it so a stale response cannot replace another editor's body.

## Related decisions

- [ADR-2026-07-24-operator-owned-agent-launcher-settings](../../../decisions/2026-07-24-operator-owned-agent-launcher-settings.md)
- [ADR-2026-09-05-trusted-browser-html-preview](../../../decisions/2026-09-05-trusted-browser-html-preview.md)
