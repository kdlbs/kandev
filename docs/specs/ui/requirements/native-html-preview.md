---
status: active
system: ui
created: 2026-09-04
owners:
  - kandev
---

# Native HTML File Preview Requirements

## Overview

Kandev shall open an `.html` or `.htm` file in a real browser runtime. The user
does not need to start a development server. The preview shall use the current
editor buffer as its entry document and shall resolve relative assets from the
task workspace. Desktop and phone surfaces replace the active file editor body
with the same proxied page while preserving a direct return to source.

HTML preview is a trusted-workspace-code feature. Previewed scripts have native
browser behavior and the authority already granted to Browser-panel content.
Kandev does not claim that this feature safely executes untrusted HTML. Users
must trust a document before they preview it.

The UI system owns the reusable file-viewer interaction. Agentctl owns the
ephemeral static server that makes current-buffer HTML and workspace files
available to the browser. The existing session port proxy owns routing and
session authorization across local and remote executors.

## Terminology

- **Entry document:** The current, possibly unsaved, editor buffer selected for
  preview.
- **Workspace preview server:** A loopback HTTP server inside agentctl that
  serves the entry document from memory and relative assets from the workspace.
- **Preview URL:** The session-scoped port-proxy URL used by the in-editor
  preview iframe or an explicitly opened Browser panel.
- **Trusted workspace code:** Content the user permits to run with the existing
  Browser-panel sandbox and origin policy. It is not isolated as hostile code.

## Requirements

### REQ-UI-NATIVE-HTML-PREVIEW-001: Preview HTML with browser fidelity

**Intent:** Let users inspect a generated or edited HTML document with normal
browser rendering, relative assets, and browser APIs. A configured
development-server command is not necessary.

#### Acceptance criteria

- **AC-UI-NATIVE-HTML-PREVIEW-001.1:** When an editable text file ends in
  `.html` or `.htm`, each file editor shall expose an accessible `Preview HTML`
  action. The action shall use each location that has the Markdown preview
  action.
- **AC-UI-NATIVE-HTML-PREVIEW-001.2:** Activating `Preview HTML` shall publish
  the current editor buffer without saving it. The source file shall remain
  open and retain its content and dirty state.
- **AC-UI-NATIVE-HTML-PREVIEW-001.3:** On desktop, activation shall replace the
  active file editor body with an iframe at the preview URL without opening a
  Browser panel. The preview shall provide `Show code`, refresh the current
  unsaved buffer on request, and offer an explicit secondary action to open the
  same URL in a Browser panel.
- **AC-UI-NATIVE-HTML-PREVIEW-001.4:** The preview shall use the native browser
  engine for markup, CSS, JavaScript, DOM events, and relative workspace assets.
  Existing Browser-panel iframe, browser, and deployment policies shall control
  links, forms, media, modules, and network APIs.
- **AC-UI-NATIVE-HTML-PREVIEW-001.5:** Relative URLs shall resolve from the
  entry document's workspace directory. The server shall not expose paths
  outside the selected task workspace or repository scope, including through
  traversal or symlink escape.
- **AC-UI-NATIVE-HTML-PREVIEW-001.6:** The preview shall use the existing
  session-authorized port-proxy path. This path shall support local, Docker,
  SSH, and Kubernetes task sessions without a second public routing mechanism.
- **AC-UI-NATIVE-HTML-PREVIEW-001.7:** Before activation, the UI shall make the
  trusted-code consequence clear. Preview source shall execute only in the
  Kandev-owned preview iframe or an explicitly opened Browser panel and shall
  never be inserted directly into the Kandev parent document.
- **AC-UI-NATIVE-HTML-PREVIEW-001.8:** On phone and coarse-pointer surfaces, the
  focused file viewer shall expose the preview action. The active touch
  dimension shall be at least 44 pixels. Preview shall occupy the full-height
  viewer, provide `Show code`, and avoid document-level horizontal overflow.
- **AC-UI-NATIVE-HTML-PREVIEW-001.9:** Adding HTML preview shall not change
  Markdown sanitization, source editing, saving, downloading, deleting,
  commenting, external-editor actions, or explicit development-server flows.
- **AC-UI-NATIVE-HTML-PREVIEW-001.10:** If the task session or agentctl is not
  available, the buffer is too large, or the server cannot start, Kandev shall
  show localized recovery copy. The UI shall preserve source access and permit
  retry. It shall not silently use another execution model.
- **AC-UI-NATIVE-HTML-PREVIEW-001.11:** Preview overlays and their server shall
  be bounded and ephemeral. They shall end with the agentctl instance and shall
  not persist file contents to Kandev storage. A stale preview URL after an
  executor restart shall be recoverable by activating `Preview HTML` again.

## Out of scope

- Safely executing hostile or untrusted HTML.
- A dedicated untrusted-content origin, network egress filtering, or browser
  capability emulation.
- Framework build pipelines, hot-module replacement, package installation, or
  replacing an application's configured development-server command.
- Rendering HTML reconstructed from a Review diff.
- Mapping rendered elements to source lines, inspector annotations, or console
  forwarding beyond existing Browser-panel behavior.
- Publishing the preview server as a production website or a durable URL.
