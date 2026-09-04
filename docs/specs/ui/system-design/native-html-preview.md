---
status: draft
system: ui
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
---
# Native HTML File Preview System Design

## Purpose and boundaries

The UI system owns HTML preview as an extension of Kandev's existing rendered
file-viewer interaction. The feature consumes the file content already held in
the frontend editor buffer. It adds no HTTP or WebSocket API, executor process,
workspace file-serving route, or database state.

The initial contract is self-contained HTML. Relative workspace assets and
remote resources are blocked rather than partially emulated. Multi-file sites
continue to use a development server and the existing Browser panel.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-NATIVE-HTML-PREVIEW-001` | [Preview eligibility and state](#preview-eligibility-and-state), [HTML document construction](#html-document-construction), [Security boundary](#security-boundary), [Responsive contract](#responsive-contract), [Compatibility and persistence](#compatibility-and-persistence) |

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
format-neutral rendered-preview boolean. Existing persisted
`markdownPreview` values are accepted as a legacy read alias during restoration
so an open Markdown preview is not lost across the change. New persistence
writes only the format-neutral field. Preview kind is always derived from the
current path rather than stored, preventing stale state from selecting the
wrong renderer after a file identity change.

Both the Dockview file-editor path and the legacy center-panel file-tab path use
the same derived preview kind and toggle semantics. A dirty buffer remains
owned by the editor state while preview is active. Returning to source remounts
the configured Monaco or CodeMirror editor with that unchanged buffer.

## Components and responsibilities

- `apps/web/lib/utils/file-types.ts` owns extension-to-preview-kind detection.
- `apps/web/components/task/file-editor-content.tsx` selects the source editor,
  `MarkdownPreviewContent`, or `HtmlPreviewContent` from preview kind and state.
- `apps/web/components/task/html-preview-content.tsx` owns safe document
  construction, the preview frame, its title, and the preview toolbar.
- Monaco and CodeMirror toolbars expose a format-neutral preview button with
  localized copy selected from preview kind.
- `apps/web/components/task/file-editor-panel.tsx`,
  `apps/web/components/task/file-tab-content.tsx`, and
  `apps/web/components/task/task-center-panel.tsx` carry format-neutral preview
  state. These components cover the two desktop file-editor paths.
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx` applies the same
  resolver and renderer inside the existing focused mobile file surface.
- `apps/web/lib/local-storage.ts` and `apps/web/hooks/use-file-editors.ts`
  preserve per-file rendered-preview state for same-session restoration.

Markdown preview keeps its comment overlays and sanitized React renderer. HTML
preview does not share the Markdown renderer, DOM, or source-line comment
mapping.

## HTML document construction

`buildHtmlPreviewDocument(content)` parses the current buffer as HTML. Before
serialization, it prepends a restrictive Content Security Policy meta element
to the document head. The policy permits only the capabilities in the
self-contained contract:

- inline styles. Script elements and inline event handlers remain inert.
- `data:` and `blob:` image or media resources.
- `data:` fonts.
- no default, object, frame, worker, manifest, connection, form, or remote
  resource source.
- no base URL override.

The builder returns a complete preview document for the iframe `srcDoc` value.
It does not sanitize or rewrite visible markup. The browser frame and CSP
contain the content. Kandev never inserts the workspace HTML into its own DOM.
Browser HTML error recovery handles incomplete documents without changing the
source buffer.

The iframe fills the preview body, has a localized accessible title and
`referrerPolicy="no-referrer"`, and receives the current buffer directly.
Because preview replaces the editor rather than rendering beside it, the frame
does not reload on source keystrokes. A workspace update that legitimately
replaces a clean buffer supplies a new `srcDoc` on the next preview render.

## Security boundary

Workspace HTML is untrusted active content. The iframe uses an empty
`sandbox` attribute and deliberately omits `allow-same-origin`, `allow-scripts`,
`allow-forms`, `allow-popups`, `allow-modals`, `allow-downloads`, and every
top-navigation capability. The result is an opaque origin even though the
`srcDoc` is created by the Kandev page.

The injected CSP is defense in depth. It blocks scripts, network APIs, remote
resources, nested frames, objects, workers, forms, and base-URL changes.
The sandbox remains the authority for script, parent-document, storage,
navigation, popup, and download isolation. Tests assert blocked script
execution and navigation. Neither the HTML document nor its inert scripts
receive task, session, repository, or credential values.

This applies the untrusted-content origin rule in
[ADR-2026-07-24-operator-owned-agent-launcher-settings](../../../decisions/2026-07-24-operator-owned-agent-launcher-settings.md).
No new ADR is required because that accepted decision already owns the durable
security boundary. This design is one consumer of it.

## Control flow

1. A file surface loads text content into its existing editor state.
2. The preview-kind resolver derives `html` from an eligible `.html` or `.htm`
   path.
3. The source toolbar exposes `Preview HTML`.
4. The toggle updates only that file's rendered-preview state.
5. `FileEditorContent` selects `HtmlPreviewContent` and passes the current
   buffer.
6. The HTML preview builder injects the CSP and supplies the document through
   iframe `srcDoc`.
7. `Show code` clears rendered-preview state and restores the configured source
   editor.

No backend request is made after the file's normal content load.

## Responsive contract

- **Desktop outcome and entry point:** Monaco and CodeMirror use the existing
  eye-icon location in the file toolbar. Preview replaces the editor inside the
  current Dockview or center-panel tab.
- **Mobile entry point:** The existing `MobileFileViewerPanel` header exposes
  the same preview action as a 44-pixel touch target. It remains visible in
  source mode. The preview toolbar exposes `Show code`.
- **Nearest shipped exemplar:** Markdown file preview supplies the desktop
  toolbar placement, same-surface transition, mobile focused viewer, preview
  header, and per-file state behavior.
- **Hierarchy and primary action:** Reading the current file remains the only
  focal task. No drawer, split view, or secondary navigation surface is added.
- **Scroll ownership:** The existing file panel remains the vertical owner and
  the iframe contains its document viewport. Kandev's page and file header do
  not gain horizontal overflow.
- **Shared versus specialized behavior:** Eligibility, state, and toggling are
  shared across viewports and editor providers. Only the existing responsive
  file-viewer composition differs.

Desktop Chromium and mobile Chrome Playwright scenarios prove the same source
to preview to source outcome. The mobile scenario also checks touch-target
geometry and document containment.

## Failure and recovery

- Invalid or incomplete HTML uses browser parsing recovery inside the sandbox.
  The toolbar stays outside the frame and always retains `Show code`.
- A blocked relative or remote resource fails inside the frame without
  triggering a workspace request or closing the preview.
- If preview document construction throws, the surface shows localized preview
  failure copy plus `Show code`. It never falls back to unsandboxed rendering.
- A restored preview flag on an ineligible path is ignored because eligibility
  is derived from the current file identity.

## Compatibility and persistence

Rendered-preview state remains transient session UI state. It is stored without
file contents in the existing per-session `sessionStorage` records so a desktop
page refresh restores the current view. There is no database or cross-session
preference.

Restoration accepts the legacy `markdownPreview` field and normalizes it to the
new format-neutral field. This is a read-only compatibility bridge. New records
do not dual-write the old name. Mobile continues to reset local preview state
when the repository-plus-path identity changes.

Markdown rendering retains `rehype-raw` followed by `rehype-sanitize` and its
existing comment behavior. HTML preview does not change the Browser panel,
Review diff preview, downloads, external VCS links, file mutations, or editor
selection.

## Verification strategy

- Unit tests cover `.html`/`.htm` eligibility, binary exclusion, generic preview
  state, and legacy restoration. They also cover CSP injection, the empty
  sandbox attribute, blocked scripts, and provider-specific toolbar copy.
- Component tests cover source/HTML/Markdown renderer selection, dirty-buffer
  preservation, failure recovery, and mobile file-identity reset.
- Desktop E2E opens an unsaved HTML buffer and previews its markup while
  attempting script and meta-refresh navigation. It verifies that neither
  navigation makes a request, returns to code, and reloads a persisted preview.
- Mobile Chrome E2E opens the focused file viewer and uses the preview action.
  It validates the output and returns to source. It verifies the touch target
  and document-level horizontal containment.

## Related decisions

- [ADR-2026-07-24-operator-owned-agent-launcher-settings](../../../decisions/2026-07-24-operator-owned-agent-launcher-settings.md)
