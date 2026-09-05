---
status: draft
system: ui
created: 2026-09-04
owners:
  - kandev
---
# Native HTML File Preview Requirements

## Overview

Kandev can render Markdown files inside its file surfaces. An HTML file
currently requires another editor or a separate development server. The UI
system owns the native HTML preview because it is a reusable file-viewer
interaction. The preview does not introduce workspace state, an executor
lifecycle, or a backend workspace-file-serving contract.

The approved slice renders the current in-memory file buffer and supports
inline JavaScript. Source code runs in a capability-free preview runtime and
never becomes executable code in the Kandev page or its credentialed browser
origin.

## Terminology

- **Rendered preview:** A view of the current file buffer that replaces the
  source editor inside the same file surface.
- **Self-contained HTML:** An `.html` or `.htm` document that uses its own
  markup, inline styles, and inline scripts. Resources are embedded as
  `data:` values or preview-runtime-owned `blob:` values. Workspace-relative
  and remote resources are not part of this contract.
- **Preview runtime:** The isolated ECMAScript execution environment that
  exposes a virtual document, bounded timers, and preview events without
  exposing browser or Kandev authority.
- **Preview surface:** The scriptless, controlled renderer that displays
  virtual-document snapshots and forwards user events to the preview runtime.

## Requirements

### REQ-UI-NATIVE-HTML-PREVIEW-001: Render HTML files in place

**Intent:** Let users inspect a generated or edited HTML document without
starting a server or leaving Kandev. The file editor remains the source of
truth.

#### Acceptance criteria

- **AC-UI-NATIVE-HTML-PREVIEW-001.1:** When an editable text file ends in
  `.html` or `.htm`, each file editor shall expose an accessible `Preview HTML`
  action. The action shall appear wherever the equivalent Markdown preview
  action is available.
- **AC-UI-NATIVE-HTML-PREVIEW-001.2:** When a user activates `Preview HTML`, the
  file surface shall replace the source editor with the current buffer's
  rendered view. The file shall stay open, and the user shall not need to save
  it. Activating `Show code` shall restore the source editor with its content
  and dirty state unchanged.
- **AC-UI-NATIVE-HTML-PREVIEW-001.3:** The rendered preview shall support HTML
  markup, inline CSS, inline JavaScript, `data:` resources, and preview-runtime-
  owned `blob:` resources. Inline scripts shall be able to mutate the virtual
  document and respond to preview events.
- **AC-UI-NATIVE-HTML-PREVIEW-001.4:** Inline JavaScript shall execute only in
  the preview runtime. It shall not access the Kandev document, browser
  origin, cookies, storage, credentials, parent or top-level windows, task or
  session data, or native browser DOM objects. The visible preview surface
  shall not execute source scripts or inline event-handler attributes.
- **AC-UI-NATIVE-HTML-PREVIEW-001.5:** Preview content shall not make outbound
  network requests. Fetch, XHR, WebSocket, EventSource, service-worker,
  external-script, external-style, and external-media capabilities shall be
  unavailable or denied. Only embedded `data:` and preview-runtime-owned
  `blob:` resources may be rendered.
- **AC-UI-NATIVE-HTML-PREVIEW-001.6:** Desktop preview state shall remain scoped
  to the open file and shall survive a same-session page refresh consistently
  with Markdown preview. A mobile file identity change shall reset the focused
  viewer to source unless preview was explicitly requested for that file.
- **AC-UI-NATIVE-HTML-PREVIEW-001.7:** On phone and coarse-pointer surfaces, the
  focused file viewer shall expose the preview action. Its active touch
  dimension shall be at least 44 pixels. The rendered document shall stay in
  the full-height surface without document-level horizontal overflow.
- **AC-UI-NATIVE-HTML-PREVIEW-001.8:** Adding HTML preview shall not change
  Markdown sanitization, source editing, saving, downloading, deleting,
  commenting, or external-editor actions for any file type.
- **AC-UI-NATIVE-HTML-PREVIEW-001.9:** A preview-initiated navigation shall not
  replace the preview, change the parent page, open a window, download a file,
  or issue a network request. This includes static and dynamically-created
  links, SVG or `xlink` links, forms, meta refresh, location or history APIs,
  and equivalent virtual-runtime actions.
- **AC-UI-NATIVE-HTML-PREVIEW-001.10:** If a script throws, requests an
  unsupported capability, exceeds the execution budget, or causes the runtime
  to terminate, the preview shall fail closed, show localized recovery copy,
  and retain a route back to source. It shall never fall back to native
  browser script execution.

## Out of scope

- Resolving workspace-relative CSS, JavaScript, images, fonts, links, or other
  files.
- Loading remote network resources from an HTML preview.
- Exposing the complete browser API, Web Workers, service workers, WebGL,
  browser storage, or credentialed platform integrations to preview scripts.
- Rendering an HTML file from a Review diff or reconstructing omitted diff
  content.
- Preview inspector annotations, source-console forwarding, or dev-server
  behavior.
- Running multi-file applications without their normal development server.
- Mapping rendered HTML elements back to source lines for review comments.
