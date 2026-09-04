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
system owns a native HTML preview because this is a reusable file-viewer
interaction. The preview does not introduce workspace state, an executor
lifecycle, or a backend serving contract.

The first supported slice is intentionally limited to self-contained static
HTML. It renders the current in-memory file buffer and isolates active content
from the Kandev application.

## Terminology

- **Rendered preview:** A view of the current file buffer that replaces the
  source editor inside the same file surface.
- **Self-contained HTML:** An `.html` or `.htm` document that uses its own
  markup and inline styles. Fonts use embedded `data:` values, while images and
  media use embedded `data:` or `blob:` values. Script elements and inline event
  handlers remain inert.
- **Preview sandbox:** The opaque-origin browser frame in which HTML content is
  rendered without Kandev application authority.

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
  markup, inline CSS, `data:` fonts, and `data:` or `blob:` images and media
  from the current document. Script elements and inline event handlers shall
  not execute.
- **AC-UI-NATIVE-HTML-PREVIEW-001.4:** Previewed HTML shall render in an
  opaque-origin sandbox with no script capability. It shall not access Kandev's
  document, browser storage, application credentials, top-level navigation,
  forms, popups, downloads, or network APIs.
- **AC-UI-NATIVE-HTML-PREVIEW-001.5:** When the document references a blocked
  network or workspace-relative resource, the preview shall remain available.
  The resource shall not load, and the user shall still be able to return to
  source.
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

## Out of scope

- Resolving workspace-relative CSS, JavaScript, images, fonts, links, or other
  files.
- Loading remote network resources from an HTML preview.
- Rendering an HTML file from a Review diff or reconstructing omitted diff
  content.
- Preview inspector annotations, console forwarding, or dev-server behavior.
- Running multi-file applications without their normal development server.
- Mapping rendered HTML elements back to source lines for review comments.
