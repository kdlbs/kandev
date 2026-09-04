---
created: 2026-09-04
status: complete
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
legacy_specs: []
---
# Implementation Plan: Native HTML File Preview

## Overview

Add a native, sandboxed HTML renderer to Kandev's existing file-preview
interaction. First generalize preview eligibility and persisted state without
changing Markdown behavior. Then connect the HTML renderer to both editor
providers and the focused mobile viewer. Finish with public guidance and
responsive browser evidence.

This order makes the security and compatibility boundaries testable before UI
surfaces depend on them. The feature stays frontend-only and renders the current
in-memory buffer. It does not add a workspace file server.

## Scope

### In scope

- `.html` and `.htm` preview eligibility for editable text files.
- A format-neutral rendered-preview state with legacy Markdown restoration.
- A self-contained HTML document builder with an injected restrictive CSP.
- An opaque-origin iframe that renders markup and inline CSS without executing
  scripts or sharing Kandev application authority.
- Preview and source toggles in Monaco, CodeMirror, the center-panel file tabs,
  Dockview file editors, and the focused mobile file viewer.
- Complete localized copy in the five shipped locales and pseudo-locale
  generation.
- Public instructions and explicit self-contained-preview limitations.
- Focused unit, component, desktop E2E, and mobile E2E coverage.

### Out of scope

- Workspace-relative or remote assets.
- Backend workspace-preview routes or a separate preview web origin.
- HTML preview reconstructed from Review diffs.
- Browser-panel inspector annotations or console forwarding.
- Multi-file application preview without a development server.
- Rendered-element source-line comments.

## Technical approach

### Preview kind and state compatibility

Extend `apps/web/lib/utils/file-types.ts` with a format-neutral preview-kind
resolver while preserving `isMarkdownFile()` for current consumers. Add a
`renderedPreview` field to `FileEditorState`, `OpenFileTab`, and
`StoredFileTab`. Normalize the legacy `markdownPreview` field on read and stop
writing it in new session-storage records.

Update `apps/web/hooks/use-file-editors.ts` and the center-panel tab helpers to
carry the generic flag through open, restore, persistence, and repo-scoped file
identity. Eligibility remains derived from the current path and binary status.
stored state cannot make an unsupported file render.

Add a pure HTML document builder under `apps/web/lib/html-preview/`. It parses
the buffer, prepends Kandev's restrictive preview CSP, and serializes a complete
document for `srcDoc`. Its tests own the exact allowed and denied policy tokens.

### Rendered file surfaces

Add `apps/web/components/task/html-preview-content.tsx` with a preview header
and a full-body iframe. The iframe uses an empty `sandbox` attribute, omits
script, same-origin, and navigation capabilities, uses
`referrerPolicy="no-referrer"`, and receives no Kandev identifiers or
credentials.

Generalize the `FileEditorContent` contract from Markdown-specific booleans and
callbacks to preview kind, preview state, and a shared toggle. Monaco and
CodeMirror retain their current toolbar placement but select localized
`Preview Markdown` or `Preview HTML` copy from preview kind. Markdown continues
to use its sanitized renderer and comment overlays.

Wire the generic state through `FileEditorPanel`, `FileTabContent`, and
`TaskCenterPanel`. Extend `MobileFileViewerPanel` using the existing focused
full-height composition, a touch-sized preview control, and file-identity reset.
Do not add a drawer, split view, or new navigation path.

### Localization

Add the HTML preview and failure labels to the `editors` or `task` namespaces
in English, Portuguese, Simplified Chinese, and both Traditional Chinese
catalogs. Generate the pseudo locale and Traditional Chinese pair with the
repository scripts rather than leaving English fallback text.

### Public guidance

Add a short how-to subsection to `docs/public/developer-tools.md` beside the
Files and editor instructions. Explain the eye action, unsaved-buffer behavior,
the sandboxed static-preview boundary, the self-contained limitation, and when
to use a dev server plus Browser panel instead.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-UI-NATIVE-HTML-PREVIEW-001.1`, `.8` | Preview-kind, toolbar, and renderer-selection Vitest coverage for HTML, Markdown, unsupported, and binary files |
| `AC-UI-NATIVE-HTML-PREVIEW-001.2`, `.6` | File-state, dirty-buffer, center-panel, Dockview, and session-storage restoration tests |
| `AC-UI-NATIVE-HTML-PREVIEW-001.3`, `.4`, `.5` | HTML document-builder and preview-frame component tests for CSP, sandbox attributes, allowed static content, blocked capabilities, and recoverable failures |
| `AC-UI-NATIVE-HTML-PREVIEW-001.7` | Focused mobile viewer component tests for entry point, identity reset, touch sizing, and containment |

## E2E tests

- `apps/web/e2e/tests/chat/html-preview.spec.ts` runs under Chromium. It covers
  `AC-UI-NATIVE-HTML-PREVIEW-001.1` through `.6` and `.8`. The test edits an
  HTML file without saving and renders its markup while attempting script and
  meta-refresh navigation. It proves that neither navigation makes a request,
  returns to source, and restores preview after a refresh.
- `apps/web/e2e/tests/task/mobile-html-preview.spec.ts` runs under
  `mobile-chrome`. It covers `AC-UI-NATIVE-HTML-PREVIEW-001.1`, `.2`, `.3`,
  `.7`, and `.8`. The test opens the focused viewer, previews the file, and
  returns to source. It verifies the touch target and zero document horizontal
  overflow.

## Work orders

- [x] [Task 01: Establish preview state and sandbox contract](task-01-preview-state-and-sandbox.md)
- [x] [Task 02: Add responsive HTML preview surfaces](task-02-responsive-html-preview.md)
- [x] [Task 03: Publish HTML preview guidance](task-03-public-html-preview-guidance.md)
- [x] [Task 04: Prove responsive HTML preview flows](task-04-responsive-html-preview-e2e.md)

## Verification results

Passed. The production web build, focused unit/component tests, localization
checks, public-doc validators, and desktop/mobile browser scenarios all pass.

## Risks

- A permissive iframe flag or weakened CSP can cross the untrusted-content
  boundary established by the accepted operator-security ADR.
- CSP behavior in `srcDoc` must remain equivalent in Chromium and the desktop
  WebView. Component assertions alone cannot prove browser enforcement.
- Two live file-editor paths and a legacy session-storage field can drift if
  the preview-state migration is applied to only one path.
- A restored preview flag must never bypass extension or binary eligibility.
- HTML documents that depend on relative or remote resources will render
  partially by design. Public guidance must make that boundary explicit.

## Open questions

None. The initial contract is self-contained `.html`/`.htm` preview only.
workspace-relative resources and Review-diff preview require later requirements
and design.
