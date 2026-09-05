---
status: draft
system: ui
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
---
# Native HTML File Preview System Design

## Purpose and boundaries

The UI system owns HTML preview as an extension of Kandev's rendered
file-viewer interaction. The feature consumes the file content already held in
the frontend editor buffer. It adds no workspace state, executor lifecycle, or
workspace-file-serving API.

Inline JavaScript is an approved preview capability, but it must not execute
as browser JavaScript in the Kandev page, an operator origin, or a credentialed
iframe. The design therefore separates three concerns:

1. A capability-free preview runtime executes source scripts against a virtual
   document.
2. A scriptless preview surface renders virtual-document snapshots.
3. A narrow event and snapshot protocol connects the two without exposing
   Kandev objects or browser authority.

The current `srcDoc` document builder and its navigation neutralization remain
useful as input normalization and defense in depth. They are not the runtime
security boundary and must not be extended by adding `allow-scripts`.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-NATIVE-HTML-PREVIEW-001` | [Preview eligibility and state](#preview-eligibility-and-state), [Runtime boundary](#runtime-boundary), [Renderer and resource policy](#renderer-and-resource-policy), [Security invariants](#security-invariants), [Responsive contract](#responsive-contract), [Failure and recovery](#failure-and-recovery), [Verification strategy](#verification-strategy) |

## Preview eligibility and state

`apps/web/lib/utils/file-types.ts` defines a small preview-kind resolver for
editable text files:

```text
.md/.mdx   -> markdown
.html/.htm -> html
other      -> none
```

Binary classification remains authoritative before preview-kind detection. An
HTML extension reported as binary is not eligible.

The file state in `FileEditorState`, `OpenFileTab`, and `StoredFileTab` uses a
format-neutral rendered-preview boolean. Existing persisted `markdownPreview`
values are accepted as a legacy read alias during restoration so an open
Markdown preview is not lost across the change. New persistence writes only
the format-neutral field. Preview kind is derived from the current path rather
than stored, preventing stale state from selecting the wrong renderer after a
file identity change.

Both the Dockview file-editor path and the legacy center-panel file-tab path use
the same derived preview kind and toggle semantics. A dirty buffer remains
owned by editor state while preview is active. Returning to source restores the
configured Monaco or CodeMirror editor with that unchanged buffer.

## Components and responsibilities

- `apps/web/lib/utils/file-types.ts` owns extension-to-preview-kind detection.
- `apps/web/hooks/use-file-editors.ts` and the file-tab helpers own generic
  preview state, restoration, and persistence.
- `apps/web/lib/html-preview/preview-runtime.ts` owns the worker client and
  validates the runtime message contract.
- `apps/web/lib/html-preview/preview-runtime.worker.ts` owns the isolated
  ECMAScript VM, virtual DOM, event dispatch, resource capability checks, and
  execution budgets.
- `apps/web/lib/html-preview/preview-runtime-types.ts` owns the structured
  request, event, snapshot, diagnostic, and failure shapes.
- `apps/web/lib/html-preview/preview-surface.ts` owns the scriptless renderer.
  It creates elements and styles through an allowlisted DOM projection and
  never inserts source HTML with `innerHTML` or `dangerouslySetInnerHTML`.
- `apps/web/lib/html-preview/html-preview-document.ts` owns source parsing and
  static navigation normalization. It removes meta refresh and non-fragment
  `href` or SVG `xlink:href` values before the runtime receives the document.
  The runtime repeats the policy for DOM mutations.
- `apps/web/components/task/html-preview-content.tsx` owns the preview header,
  runtime lifecycle, scriptless surface, localized failure state, and source
  toggle.
- Monaco, CodeMirror, Dockview, center-panel, and mobile file-viewer
  components select the same runtime-backed HTML renderer. They do not create
  a second execution path.

Markdown preview keeps its sanitized React renderer, comment overlays, and
  source-line behavior. HTML preview does not share the Markdown renderer or
  comment DOM.

## Runtime boundary

### Execution host

`PreviewRuntimeWorker` runs in a dedicated Web Worker. It loads a pinned
ECMAScript engine with no host imports and no direct access to the worker's
native global object. The engine receives a virtual `window` and `document`
  object implemented by the runtime, not references to browser objects.

The implementation must execute all source JavaScript through that engine. It
must not use native `eval`, `Function`, script elements, inline event-handler
attributes, or an iframe with `allow-scripts` to execute source code.

The virtual capability surface is intentionally small:

- DOM nodes, attributes, text, style values, and document mutation;
- document-ready and user-event dispatch;
- bounded timers and microtasks;
- `console` diagnostics that return to the host without source content in
  production logs; and
- runtime-owned `Blob` and `URL.createObjectURL` values whose bytes originate
  from the preview source or VM.

The runtime does not expose native `window`, `location`, `history`, `parent`,
`top`, `frames`, `navigator`, cookies, local or session storage, IndexedDB,
cache storage, service workers, Web Workers, WebSockets, `fetch`, XHR,
EventSource, `sendBeacon`, downloads, or window-opening APIs. Unsupported APIs
fail closed with a virtual runtime error.

### Message contract

The worker client uses a structured-clone contract with no callback or object
reference crossing the boundary:

```text
Load(source: string) -> Ready(snapshot) | Failed(diagnostics)
Dispatch(event: PreviewEvent) -> Snapshot | Failed(diagnostics)
Dispose() -> terminated worker
```

`PreviewEvent` contains only a supported event type, a virtual node identity,
and sanitized primitive event data. It never contains a DOM node, task ID,
session ID, repository path, boot payload, cookie, credential, or arbitrary
function. Snapshots contain virtual nodes, safe attributes, safe styles,
runtime-owned resource tokens, and diagnostics. They do not contain executable
source or host object references.

The worker receives only the current file buffer and user events. It never
receives the Kandev boot payload or data from stores outside the file buffer.

### Execution limits

Every load and event turn has a wall-clock and instruction budget. The worker
also has a bounded virtual heap, timer count, snapshot size, and event queue.
The VM must expose an interrupt or equivalent termination hook so an infinite
loop cannot block the application. Exceeding any limit terminates the worker,
discards its virtual state, and reports a recoverable preview failure.

## Renderer and resource policy

The visible preview surface is scriptless. The preferred implementation is a
Shadow DOM surface mounted by `HtmlPreviewContent`; a scriptless iframe is not
an alternate execution path. The renderer builds native elements from the
virtual snapshot and attaches event delegation owned by Kandev. Source scripts
and inline event-handler attributes are never placed in the browser DOM.

The renderer applies a deny-by-default URL policy:

- `data:` resources are allowed for the resource element types covered by the
  requirements.
- `blob:` resources are allowed only when the runtime created the token from
  source-owned or VM-owned bytes.
- HTTP, HTTPS, protocol-relative, workspace-relative, `javascript:`, `file:`,
  and unknown URLs are omitted or rendered as inert values.
- Anchor, area, SVG link, form, meta-refresh, download, and window-opening
  actions are not given native default behavior.
- CSS `url()` values are filtered by the same resource policy before a style
  is adopted into the preview surface.
- Nested frames, objects, plugins, and workers are omitted or replaced by an
  inert placeholder.

The existing navigation neutralizer runs before the source is parsed by the
runtime. The virtual DOM mutation layer enforces the same rule after scripts
create or modify nodes. This prevents a script from reintroducing a link after
the initial parse.

The renderer uses containment and scoped styles so preview markup cannot style
or traverse the surrounding Kandev UI. It does not rely on CSP's unsupported
navigation directives, iframe `sandbox` flags, or parent `load` handlers as
the only enforcement mechanism.

## Security invariants

Workspace HTML is untrusted active content. The following invariants are
required before this feature can ship:

1. No untrusted source executes in the Kandev page, a native browser script
   context, or an `allow-scripts` iframe.
2. The VM has no host imports that reach the browser, Kandev stores, cookies,
   credentials, filesystem, network, or parent document.
3. The renderer receives data-only snapshots and never evaluates or inserts
   source-controlled executable markup.
4. All static and dynamic resource URLs pass one deny-by-default policy.
5. All static and dynamic navigation actions are no-ops and produce no
   browser navigation request.
6. A worker failure, budget exhaustion, malformed message, or unsupported API
   fails closed and never falls back to native script execution.
7. The worker is terminated when the preview closes, the file changes identity,
   the page leaves the file surface, or a runtime generation is superseded.

This design applies the untrusted-content origin rule in
[ADR-2026-07-24-operator-owned-agent-launcher-settings](../../../decisions/2026-07-24-operator-owned-agent-launcher-settings.md).
The execution-specific choice is recorded in the proposed
[capability-free preview runtime ADR](../../../decisions/2026-09-05-script-capable-html-preview-isolation.md).

## Control flow

1. A file surface loads text content into its existing editor state.
2. The preview-kind resolver derives `html` from an eligible `.html` or `.htm`
   path.
3. The source toolbar exposes `Preview HTML`.
4. The toggle starts a new runtime generation and sends only the current
   buffer to the worker.
5. The worker parses the document, normalizes navigation, extracts inline
   scripts and handlers, executes them in order in the virtual environment,
   and emits a snapshot.
6. The scriptless surface renders the snapshot and registers event delegation.
7. A supported user event becomes a sanitized `Dispatch` message. The worker
   runs handlers and emits the next snapshot.
8. `Show code`, file identity changes, runtime failures, and page teardown
   dispose the worker and restore or retain the source editor according to the
   existing file-state contract.

No backend request is made for execution. The only normal content input is the
file buffer already loaded by the Files surface.

## Responsive contract

- **Desktop outcome and entry point:** Monaco and CodeMirror use the existing
  eye-icon location in the file toolbar. Preview replaces the editor inside
  the current Dockview or center-panel tab.
- **Mobile entry point:** The existing `MobileFileViewerPanel` header exposes
  the same preview action as a 44-pixel touch target. It remains visible in
  source mode. The preview toolbar exposes `Show code` and runtime failure
  recovery.
- **Nearest shipped exemplar:** Markdown file preview supplies toolbar
  placement, same-surface transition, mobile focused viewer, and per-file
  state behavior.
- **Hierarchy and primary action:** Reading the current file remains the only
  focal task. No drawer, split view, or secondary navigation surface is added.
- **Scroll ownership:** The existing file panel remains the vertical owner.
  The controlled preview surface owns document scrolling and does not add
  horizontal overflow to Kandev's page or file header.
- **Shared versus specialized behavior:** Eligibility, state, runtime policy,
  and toggling are shared across viewports and editor providers. Only the
  existing responsive file-viewer composition differs.

## Failure and recovery

- Invalid or incomplete HTML uses runtime parser recovery. The toolbar stays
  outside the preview surface and always retains `Show code`.
- A script exception or unsupported API produces a localized diagnostic while
  retaining the last safe snapshot when possible. No native execution fallback
  is permitted.
- A loop, memory pressure event, oversized snapshot, or malformed worker
  message terminates the runtime generation and shows localized recovery copy.
- A blocked relative or remote resource becomes inert without a workspace or
  external request.
- A blocked static or dynamic navigation is a no-op. The preview remains on
  the current snapshot and the parent page URL is unchanged.
- If the runtime cannot initialize, the surface shows localized preview
  failure copy and a route to source. It never renders the source through an
  unsandboxed browser path.

## Compatibility and persistence

Rendered-preview state remains transient session UI state. It is stored without
file contents in the existing per-session `sessionStorage` records so a desktop
page refresh restores the current view. There is no database or cross-session
preference.

Restoration accepts the legacy `markdownPreview` field and normalizes it to the
new format-neutral field. New records do not dual-write the old name. Mobile
continues to reset local preview state when the repository-plus-path identity
changes.

Runtime generations are not persisted. A restored HTML preview starts a fresh
worker from the restored current buffer; it does not restore script memory,
timers, event queues, or runtime-owned resource tokens.

## Observability

The runtime reports structured local diagnostics for initialization failures,
unsupported capabilities, budget exhaustion, and worker termination. Logs do
not include source HTML, script bodies, task identifiers, or credentials.
Development builds may expose bounded runtime counters and sanitized console
messages. Production behavior remains fail closed and user-visible through
localized preview recovery copy.

## Verification strategy

- Runtime unit tests prove that inline scripts mutate the virtual document,
  event handlers receive virtual events, and the worker never exposes native
  browser objects or host imports.
- Resource-policy tests cover static and dynamically-created `href`, `src`,
  `srcset`, CSS `url()`, form, meta-refresh, SVG `xlink`, and runtime-owned
  `blob:` values.
- Component tests cover runtime lifecycle, data-only snapshot rendering,
  source restoration, failure recovery, generation disposal, and mobile file
  identity changes.
- Desktop Chromium E2E proves an inline script changes visible output and a
  user event changes virtual state. It also attempts fetch, location changes,
  dynamic links, meta refresh, form submission, and window opening, and proves
  that the preview remains visible with no child-frame or external requests.
- Mobile Chrome E2E proves the same script-capable preview entry point, touch
  target, source recovery, and document containment.
- Desktop WebView smoke coverage must repeat the navigation and network denial
  assertions before release because browser and WebView enforcement differ.

## Open design risks

- The ECMAScript engine and virtual-DOM compatibility surface must be selected
  and pinned without granting host imports. Engine size and WebView worker
  support need measurement.
- The renderer must preserve useful HTML and CSS behavior without using an
  unsafe HTML sink. Its URL and CSS policy needs adversarial tests.
- Runtime-owned `blob:` values need explicit byte ownership and cleanup so a
  source cannot reference an unrelated Kandev blob URL.
- Execution budgets need values that stop denial-of-service scripts without
  making ordinary inline interactions unusable.
- The owner must review the runtime boundary and its browser/WebView evidence
  before production implementation begins.

## Related decisions

- [ADR-2026-07-24-operator-owned-agent-launcher-settings](../../../decisions/2026-07-24-operator-owned-agent-launcher-settings.md)
- [ADR-2026-09-05-script-capable-html-preview-isolation](../../../decisions/2026-09-05-script-capable-html-preview-isolation.md)
